package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/slack-go/slack"
)

// convert string to float to string for formatting
func formatNumberWithoutDecimals(s *string) string {
	f, _ := strconv.ParseFloat(*s, 64)
	return fmt.Sprintf("%.0f", f)
}

type usageData struct {
	ServiceName string
	Amount      string
}

// get AWS Usage for specific month sorted by AWS Service Names
func getUsage(ce *costexplorer.Client, date time.Time) ([]usageData, error) {

	firstDayOfMonth := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
	firstDayOfNextMonth := firstDayOfMonth.AddDate(0, 1, 0)

	var usage []usageData
	resp, err := ce.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{
		Granularity: types.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod: &types.DateInterval{
			End:   &[]string{firstDayOfNextMonth.Format("2006-01-02")}[0],
			Start: &[]string{firstDayOfMonth.Format("2006-01-02")}[0],
		},
		GroupBy: []types.GroupDefinition{{
			Key:  aws.String("SERVICE"),
			Type: types.GroupDefinitionTypeDimension,
		}},
	})

	if err != nil {
		return usage, err
	}

	monthData := resp.ResultsByTime[0].Groups
	sort.Slice(monthData, func(i, j int) bool {
		a, _ := strconv.ParseFloat(*monthData[i].Metrics["UnblendedCost"].Amount, 64)
		b, _ := strconv.ParseFloat(*monthData[j].Metrics["UnblendedCost"].Amount, 64)
		return a > b
	})
	for _, service := range monthData {
		cost := formatNumberWithoutDecimals(service.Metrics["UnblendedCost"].Amount)
		usage = append(usage, usageData{ServiceName: service.Keys[0], Amount: cost})
	}
	return usage, nil
}

// get summary usage for day range
func getDailySummary(ce *costexplorer.Client, startDate, endDate time.Time) ([]string, error) {

	var summary []string
	resp, err := ce.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{
		Granularity: types.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod: &types.DateInterval{
			End:   &[]string{endDate.Format("2006-01-02")}[0],
			Start: &[]string{startDate.Format("2006-01-02")}[0],
		},
	})
	if err != nil {
		return summary, err
	}

	for _, day := range resp.ResultsByTime {
		cost := formatNumberWithoutDecimals(day.Total["UnblendedCost"].Amount)
		summary = append(summary, cost)
	}
	return summary, nil
}

// return total cost for one day
func getSummaryForDay(ce *costexplorer.Client, date time.Time) (string, error) {
	var summary []string
	day := date
	dayBefore := date.AddDate(0, 0, -1)
	summary, err := getDailySummary(ce, dayBefore, day)
	if err != nil {
		return "N/A", err
	}
	return summary[0], nil
}

// return total cost change for day (in percents)
func getChangeForDay(ce *costexplorer.Client, date time.Time) (string, error) {

	summaryForDay, err := getSummaryForDay(ce, date)
	if err != nil {
		return "", err
	}
	summaryForDayBefore, err := getSummaryForDay(ce, date.AddDate(0, 0, -1))
	if err != nil {
		return "", err
	}

	return getChange(summaryForDay, summaryForDayBefore), nil
}

// return average cost for 7 days from specific date
func getAvgSummaryForSevenDays(ce *costexplorer.Client, date time.Time) (string, error) {
	var summary []string
	var total float64
	targetDate := date
	sevenDaysBefore := date.AddDate(0, 0, -7)
	summary, err := getDailySummary(ce, sevenDaysBefore, targetDate)
	if err != nil {
		return "N/A", err
	}
	total = 0
	for _, day := range summary {
		f, _ := strconv.ParseFloat(day, 64)
		total += f
	}
	return fmt.Sprintf("%.0f", total/float64(7)), nil
}

// return average cost change for 7 days (in percents)
func getChangeForSevenDays(ce *costexplorer.Client, date time.Time) (string, error) {

	avgSummary, err := getAvgSummaryForSevenDays(ce, date)
	if err != nil {
		return "", err
	}
	avgBeforeSummary, err := getAvgSummaryForSevenDays(ce, date.AddDate(0, 0, -1))
	if err != nil {
		return "", err
	}

	return getChange(avgSummary, avgBeforeSummary), nil
}

// return average cost for last 30 days form specific date
func getAvgSummaryFor30Days(ce *costexplorer.Client, date time.Time) (string, error) {
	var summary []string
	var total float64
	targetDate := date
	thirtyDaysBefore := date.AddDate(0, 0, -30)
	summary, err := getDailySummary(ce, thirtyDaysBefore, targetDate)
	if err != nil {
		return "N/A", err
	}
	total = 0
	for _, day := range summary {
		f, _ := strconv.ParseFloat(day, 64)
		total += f
	}
	return fmt.Sprintf("%.0f", total/float64(30)), nil
}

// return average cost change for 30 days (in percents)
func getChangeFor30Days(ce *costexplorer.Client, date time.Time) (string, error) {

	avgSummary, err := getAvgSummaryFor30Days(ce, date)
	if err != nil {
		return "", err
	}
	avgBeforeSummary, err := getAvgSummaryFor30Days(ce, date.AddDate(0, 0, -1))
	if err != nil {
		return "", err
	}

	return getChange(avgSummary, avgBeforeSummary), nil
}

// return total cost for specific month
func getSummaryForMonth(ce *costexplorer.Client, year int, month time.Month) (string, error) {
	var summary string
	start := time.Date(year, month, int(1), int(0), int(0), int(0), int(0), time.UTC)
	end := start.AddDate(0, 1, 0)

	resp, err := ce.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{
		Granularity: types.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod: &types.DateInterval{
			End:   &[]string{end.Format("2006-01-02")}[0],
			Start: &[]string{start.Format("2006-01-02")}[0],
		},
	})

	if err != nil {
		return summary, err
	}

	summary = formatNumberWithoutDecimals(resp.ResultsByTime[0].Total["UnblendedCost"].Amount)
	return summary, nil
}

// return estimate cost for specific month
func getEstimateForMonth(ce *costexplorer.Client, date time.Time) (string, error) {
	var estimate string

	firstDayOfMonth := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
	end := firstDayOfMonth.AddDate(0, 1, 0)

	resp, err := ce.GetCostForecast(context.Background(), &costexplorer.GetCostForecastInput{
		Granularity: types.GranularityMonthly,
		Metric:      types.MetricUnblendedCost,
		TimePeriod: &types.DateInterval{
			End:   &[]string{end.Format("2006-01-02")}[0],
			Start: &[]string{date.Format("2006-01-02")}[0],
		},
	})

	if err != nil {
		return estimate, err
	}

	estimate = formatNumberWithoutDecimals(resp.Total.Amount)
	return estimate, nil
}

// return change in percents between current and previous values
func getChange(current, previous string) string {
	change := ""
	fcurrent, _ := strconv.ParseFloat(current, 64)
	fprevious, _ := strconv.ParseFloat(previous, 64)
	diff := int(100 * fcurrent / fprevious)
	if diff >= 100 {
		change = fmt.Sprintf("+%d%%", diff-100)
	} else {
		change = fmt.Sprintf("-%d%%", 100-diff)
	}
	return change
}

func main() {

	// check necessary environment variables SLACK_TOKEN, SLACK_CHANNEL_ID, SLACK_MESSAGE_HEADER
	// Slack app token
	slackToken, ok := os.LookupEnv("SLACK_TOKEN")
	if !ok {
		log.Fatal("Missing SLACK_TOKEN in environment")
	}
	// Slack channel id
	slackChannel, ok := os.LookupEnv("SLACK_CHANNEL_ID")
	if !ok {
		log.Fatal("Missing SLACK_CHANNEL_ID in environment")
	}
	// header text for Slack message
	slackMessageHeader, ok := os.LookupEnv("SLACK_MESSAGE_HEADER")
	if !ok {
		slackMessageHeader = "AWS account Cost and Usage details :moneybag:"
	}

	// Configure AWS SDK
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("failed to load AWS configuration, %v", err)
	}

	// AWS STS client
	stsClient := sts.NewFromConfig(cfg)
	stsData, err := stsClient.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
	if err != nil {
		log.Fatal(err)
	}
	awsAccount := *stsData.Account
	log.Printf("AWS Account: %s", awsAccount)

	// AWS Cost Explorer client
	ce := costexplorer.NewFromConfig(cfg)

	now := time.Now().UTC()

	// Daily usage details
	log.Printf("getting summary daily cost")

	daySummary, err := getSummaryForDay(ce, now)
	if err != nil {
		log.Fatal(err)
	}
	dailyChange, err := getChangeForDay(ce, now)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("daily summary cost: %s $%s (%s)", now.AddDate(0, 0, -1).Format("January 2"), daySummary, dailyChange)

	sevenDaysAvg, err := getAvgSummaryForSevenDays(ce, now)
	if err != nil {
		log.Fatal(err)
	}
	sevenDaysChange, err := getChangeForSevenDays(ce, now)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("7-days average cost: $%s (%s)", sevenDaysAvg, sevenDaysChange)

	thirtyDaysAvg, err := getAvgSummaryFor30Days(ce, now)
	if err != nil {
		log.Fatal(err)
	}
	thirtyDaysChange, err := getChangeFor30Days(ce, now)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("30-days average cost: $%s (%s)", thirtyDaysAvg, thirtyDaysChange)

	// Monthly usage details
	log.Printf("getting summary monthly cost")

	thisMonth, err := getSummaryForMonth(ce, now.Year(), now.Month())
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("current month usage: $%s", thisMonth)

	prevMonth, err := getSummaryForMonth(ce, now.AddDate(0, -1, 0).Year(), now.AddDate(0, -1, 0).Month())
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("previous month usage: $%s", prevMonth)

	estimate, err := getEstimateForMonth(ce, now)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("estimate for current month: $%s (%s)", estimate, getChange(estimate, prevMonth))

	// Monthly usage by AWS Services
	log.Printf("getting usage from AWS services")

	usage, err := getUsage(ce, now)
	if err != nil {
		log.Fatal(err)
	}
	for _, s := range usage {
		if s.Amount != "0" {
			log.Printf("service: '%s', cost: $%s", s.ServiceName, s.Amount)
		}
	}

	// Slack client
	sc := slack.New(
		slackToken,
		slack.OptionDebug(false),
		slack.OptionLog(log.New(os.Stdout, "", log.Lshortfile|log.LstdFlags)),
	)

	// message header
	headerText := slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*%s*\naccount id: `%s`", slackMessageHeader, awsAccount), false, false)
	headerSection := slack.NewSectionBlock(headerText, nil, nil)
	msgBlocks := []slack.Block{
		headerSection,
	}

	// message attachment with daily usage details
	attachmentDaily := slack.Attachment{
		Title: "Daily Summary:",
		Color: "good",
		Fields: []slack.AttachmentField{
			slack.AttachmentField{
				Title: now.AddDate(0, 0, -1).Format("January 2"),
				Value: fmt.Sprintf("$%s (%s)", daySummary, dailyChange),
				Short: true,
			},
			slack.AttachmentField{
				Title: "7-Day Avg.",
				Value: fmt.Sprintf("$%s (%s)", sevenDaysAvg, sevenDaysChange),
				Short: true,
			},
			slack.AttachmentField{
				Title: "30-Day Avg.",
				Value: fmt.Sprintf("$%s (%s)", thirtyDaysAvg, thirtyDaysChange),
				Short: true,
			},
		},
	}

	// message attachment with monthly usage details
	attachmentMonthly := slack.Attachment{
		Title: "Monthly Summary:",
		Color: "#3AA3E3",
		Fields: []slack.AttachmentField{
			slack.AttachmentField{
				Title: fmt.Sprintf("Current (%s)", now.Month()),
				Value: fmt.Sprintf("$%s", thisMonth),
				Short: true,
			},
			slack.AttachmentField{
				Title: fmt.Sprintf("Previous (%s)", now.AddDate(0, -1, 0).Month()),
				Value: fmt.Sprintf("$%s", prevMonth),
				Short: true,
			},
			slack.AttachmentField{
				Title: fmt.Sprintf("Estimate for %s", now.Month()),
				Value: fmt.Sprintf("$%s (%s)", estimate, getChange(estimate, prevMonth)),
				Short: true,
			},
		},
	}

	// message attachment with top5 AWS Services cost details
	fields := []slack.AttachmentField{}
	size := len(usage)
	if size > 5 {
		size = 5
	}
	for _, s := range usage[:size] {
		fields = append(fields, slack.AttachmentField{
			Title: s.ServiceName,
			Value: fmt.Sprintf("$%s", s.Amount),
			Short: false,
		})
	}
	attachmentTop5 := slack.Attachment{
		Title:  fmt.Sprintf("Top %d AWS services by Cost in %s:", size, now.Month()),
		Fields: fields,
	}

	// send message to Slack
	_, _, err = sc.PostMessage(
		slackChannel,
		slack.MsgOptionBlocks(msgBlocks...),
		slack.MsgOptionAttachments(attachmentDaily, attachmentMonthly, attachmentTop5),
		slack.MsgOptionAsUser(true),
	)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("message successfully sent to Slack channel %s", slackChannel)

} // main
