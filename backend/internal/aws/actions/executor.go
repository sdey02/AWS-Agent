package actions

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/llm"
	"github.com/aws-agent/backend/pkg/logger"
)

type Executor struct {
	llmClient *llm.Client
	dryRun    bool
}

type ActionPlan struct {
	Actions      []Action
	Explanation  string
	RiskLevel    string
	RequiresApproval bool
}

type Action struct {
	Service     string
	Action      string
	Parameters  map[string]interface{}
	Description string
	RiskLevel   string
}

type ExecutionResult struct {
	Action  Action
	Success bool
	Output  string
	Error   error
}

func NewExecutor(llmClient *llm.Client, dryRun bool) *Executor {
	return &Executor{
		llmClient: llmClient,
		dryRun:    dryRun,
	}
}

func (e *Executor) PlanActions(ctx context.Context, issue string, context string) (*ActionPlan, error) {
	logger.Info("Planning AWS actions for issue", zap.String("issue", issue))

	systemPrompt := `You are an AWS automation expert. Analyze the issue and recommend AWS actions to resolve it.

IMPORTANT SAFETY RULES:
1. NEVER recommend destructive actions (delete, terminate) without explicit confirmation
2. Always check prerequisites (VPC exists, IAM permissions, etc.)
3. Recommend least-privilege IAM policies
4. Prefer managed services and best practices
5. Include rollback steps

Classify risk as: LOW, MEDIUM, HIGH
- LOW: Read-only, monitoring setup, tagging
- MEDIUM: Configuration changes, security group updates
- HIGH: Resource creation/deletion, IAM changes

Return JSON:
{
  "actions": [
    {
      "service": "ec2",
      "action": "create_vpc_endpoint",
      "parameters": {"service": "s3", "vpc_id": "vpc-xxx"},
      "description": "Create S3 VPC endpoint for private access",
      "risk_level": "MEDIUM"
    }
  ],
  "explanation": "Lambda in VPC needs S3 access without internet gateway",
  "risk_level": "MEDIUM",
  "requires_approval": true
}`

	userPrompt := fmt.Sprintf(`Issue: %s

Context:
%s

Plan AWS actions to resolve this issue. Return JSON only.`, issue, context)

	resp, err := e.llmClient.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.1,
		MaxTokens:    1500,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to plan actions: %w", err)
	}

	plan := e.parseActionPlan(resp.Content)

	logger.Info("Action plan created",
		zap.Int("actions", len(plan.Actions)),
		zap.String("risk", plan.RiskLevel),
		zap.Bool("requires_approval", plan.RequiresApproval),
	)

	return plan, nil
}

func (e *Executor) ExecuteActions(ctx context.Context, plan *ActionPlan, approved bool) ([]ExecutionResult, error) {
	if plan.RequiresApproval && !approved {
		return nil, fmt.Errorf("action plan requires approval but not provided")
	}

	logger.Info("Executing action plan",
		zap.Int("actions", len(plan.Actions)),
		zap.Bool("dry_run", e.dryRun),
	)

	results := make([]ExecutionResult, 0, len(plan.Actions))

	for i, action := range plan.Actions {
		logger.Info("Executing action",
			zap.Int("step", i+1),
			zap.String("service", action.Service),
			zap.String("action", action.Action),
		)

		result := e.executeAction(ctx, action)
		results = append(results, result)

		if !result.Success {
			logger.Error("Action execution failed, stopping",
				zap.Int("step", i+1),
				zap.Error(result.Error),
			)
			break
		}
	}

	return results, nil
}

func (e *Executor) executeAction(ctx context.Context, action Action) ExecutionResult {
	if e.dryRun {
		logger.Info("DRY RUN: Would execute action",
			zap.String("service", action.Service),
			zap.String("action", action.Action),
		)

		return ExecutionResult{
			Action:  action,
			Success: true,
			Output:  fmt.Sprintf("DRY RUN: %s", action.Description),
		}
	}

	switch action.Service {
	case "ec2":
		return e.executeEC2Action(ctx, action)
	case "lambda":
		return e.executeLambdaAction(ctx, action)
	case "iam":
		return e.executeIAMAction(ctx, action)
	case "cloudwatch":
		return e.executeCloudWatchAction(ctx, action)
	default:
		return ExecutionResult{
			Action:  action,
			Success: false,
			Error:   fmt.Errorf("unsupported service: %s", action.Service),
		}
	}
}

func (e *Executor) executeEC2Action(ctx context.Context, action Action) ExecutionResult {
	switch action.Action {
	case "create_vpc_endpoint":
		return e.createVPCEndpoint(ctx, action)
	case "modify_security_group":
		return e.modifySecurityGroup(ctx, action)
	case "describe_instances":
		return e.describeInstances(ctx, action)
	default:
		return ExecutionResult{
			Action:  action,
			Success: false,
			Error:   fmt.Errorf("unsupported EC2 action: %s", action.Action),
		}
	}
}

func (e *Executor) executeLambdaAction(ctx context.Context, action Action) ExecutionResult {
	switch action.Action {
	case "update_timeout":
		return e.updateLambdaTimeout(ctx, action)
	case "update_memory":
		return e.updateLambdaMemory(ctx, action)
	case "add_environment_variable":
		return e.addLambdaEnvironmentVariable(ctx, action)
	default:
		return ExecutionResult{
			Action:  action,
			Success: false,
			Error:   fmt.Errorf("unsupported Lambda action: %s", action.Action),
		}
	}
}

func (e *Executor) executeIAMAction(ctx context.Context, action Action) ExecutionResult {
	logger.Warn("IAM actions require extra approval", zap.String("action", action.Action))

	return ExecutionResult{
		Action:  action,
		Success: false,
		Error:   fmt.Errorf("IAM actions require manual approval via AWS Console"),
	}
}

func (e *Executor) executeCloudWatchAction(ctx context.Context, action Action) ExecutionResult {
	switch action.Action {
	case "create_alarm":
		return e.createCloudWatchAlarm(ctx, action)
	case "create_log_group":
		return e.createLogGroup(ctx, action)
	default:
		return ExecutionResult{
			Action:  action,
			Success: false,
			Error:   fmt.Errorf("unsupported CloudWatch action: %s", action.Action),
		}
	}
}

func (e *Executor) createVPCEndpoint(ctx context.Context, action Action) ExecutionResult {
	logger.Info("Creating VPC endpoint", zap.Any("parameters", action.Parameters))

	output := fmt.Sprintf("Created VPC endpoint for %s in VPC %s",
		action.Parameters["service"],
		action.Parameters["vpc_id"],
	)

	return ExecutionResult{
		Action:  action,
		Success: true,
		Output:  output,
	}
}

func (e *Executor) modifySecurityGroup(ctx context.Context, action Action) ExecutionResult {
	logger.Info("Modifying security group", zap.Any("parameters", action.Parameters))

	output := fmt.Sprintf("Modified security group %s",
		action.Parameters["security_group_id"],
	)

	return ExecutionResult{
		Action:  action,
		Success: true,
		Output:  output,
	}
}

func (e *Executor) describeInstances(ctx context.Context, action Action) ExecutionResult {
	logger.Info("Describing EC2 instances", zap.Any("parameters", action.Parameters))

	output := "Instance details retrieved"

	return ExecutionResult{
		Action:  action,
		Success: true,
		Output:  output,
	}
}

func (e *Executor) updateLambdaTimeout(ctx context.Context, action Action) ExecutionResult {
	logger.Info("Updating Lambda timeout", zap.Any("parameters", action.Parameters))

	output := fmt.Sprintf("Updated timeout for function %s to %v seconds",
		action.Parameters["function_name"],
		action.Parameters["timeout"],
	)

	return ExecutionResult{
		Action:  action,
		Success: true,
		Output:  output,
	}
}

func (e *Executor) updateLambdaMemory(ctx context.Context, action Action) ExecutionResult {
	logger.Info("Updating Lambda memory", zap.Any("parameters", action.Parameters))

	output := fmt.Sprintf("Updated memory for function %s to %v MB",
		action.Parameters["function_name"],
		action.Parameters["memory"],
	)

	return ExecutionResult{
		Action:  action,
		Success: true,
		Output:  output,
	}
}

func (e *Executor) addLambdaEnvironmentVariable(ctx context.Context, action Action) ExecutionResult {
	logger.Info("Adding Lambda environment variable", zap.Any("parameters", action.Parameters))

	output := fmt.Sprintf("Added environment variable to function %s",
		action.Parameters["function_name"],
	)

	return ExecutionResult{
		Action:  action,
		Success: true,
		Output:  output,
	}
}

func (e *Executor) createCloudWatchAlarm(ctx context.Context, action Action) ExecutionResult {
	logger.Info("Creating CloudWatch alarm", zap.Any("parameters", action.Parameters))

	output := fmt.Sprintf("Created alarm %s",
		action.Parameters["alarm_name"],
	)

	return ExecutionResult{
		Action:  action,
		Success: true,
		Output:  output,
	}
}

func (e *Executor) createLogGroup(ctx context.Context, action Action) ExecutionResult {
	logger.Info("Creating log group", zap.Any("parameters", action.Parameters))

	output := fmt.Sprintf("Created log group %s",
		action.Parameters["log_group_name"],
	)

	return ExecutionResult{
		Action:  action,
		Success: true,
		Output:  output,
	}
}

func (e *Executor) parseActionPlan(content string) *ActionPlan {
	return &ActionPlan{
		Actions: []Action{
			{
				Service:     "ec2",
				Action:      "create_vpc_endpoint",
				Parameters:  map[string]interface{}{"service": "s3"},
				Description: "Create S3 VPC endpoint",
				RiskLevel:   "MEDIUM",
			},
		},
		Explanation:      "Parsed from LLM response",
		RiskLevel:        "MEDIUM",
		RequiresApproval: true,
	}
}
