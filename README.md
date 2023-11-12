# AWS Cost Reporter

Posts daily AWS cost data to a Slack channel.

<img src="aws-cost-reporter.jpg" width="450">

## Test run

See Slack Api [documentation](https://api.slack.com/start/quickstart) how to create own Slack App

### Pre-requirements

Export necessary environment variables about Slack

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

### Create an IAM role that will be used by aws-cost-reporter (see IRSA for more details). Terraform example:

```hcl
module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 19.0"

  <eks cluster orchestration details skipped>
}

data "aws_iam_policy_document" "aws_cost_reporter" {
  statement {
    sid    = "AWSCostReporter"
    effect = "Allow"

    actions = [
      "ce:GetCostAndUsage",
      "ce:GetCostForecast",
      "sts:GetCallerIdentity",
    ]

    resources = ["*"]
  }
}

resource "aws_iam_policy" "aws_cost_reporter" {
  name_prefix = "aws-cost-reporter-"
  path        = "/"
  policy      = data.aws_iam_policy_document.aws_cost_reporter.json
}

module "aws_cost_reporter_irsa" {
  source      = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version     = "5.30.2"
  create_role = true

  role_name_prefix = "aws-cost-reporter-"
  role_policy_arns = {
    policy = try(aws_iam_policy.aws_cost_reporter.arn, null)
  }

  oidc_providers = {
    this_eks_cluster = {
      provider_arn               = module.eks.oidc_provider_arn
      namespace_service_accounts = ["default:aws-cost-reporter"]
    }
  }
}

output "aws_cost_reporter_role" {
  value = module.aws_cost_reporter_irsa.iam_role_arn
}
```

### Add neondatabase helm chart repository

```console
$ helm repo add neondatabase https://neondatabase.github.io/helm-charts
```

### Install aws-cost-reporter helm chart

```console
$ helm install aws-cost-reporter neondatabase/aws-cost-reporter \
  --set slack.token="<Slack App token>" \
  --set serviceAccount.roleArn="arn:aws:iam::<AWS account id>:role/<IRSA role name>"
```

See aws-cost-reporter helm chart [docs](https://github.com/neondatabase/helm-charts/tree/main/charts/aws-cost-reporter)
for other settings
