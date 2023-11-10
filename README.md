# AWS Cost Reporter

Posts daily AWS cost data to a Slack channel.

![image](aws-cost-reporter.jpg "AWS Cost Reporter")

## Test run

### Pre-requirements

Export necessary environtment variables about Slack

```consile
export SLACK_TOKEN="xoxb-1234567890123-123..."
export SLACK_CHANNEL_ID="#test-channel"
```

### by using AWS credentials

```console
AWS_ACCESS_KEY_ID="YOURKEYHERE" AWS_SECRET_ACCESS_KEY="YourSecretHere" AWS_REGION="us-east-1" go run main.go
```

### by using AWS profile

```console
AWS_PROFILE=dev go run main.go
```

## Run in EKS cluster

TODO

