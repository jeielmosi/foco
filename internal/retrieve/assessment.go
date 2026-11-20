package retrieve

import (
	"context"

	"foco/internal/enums"
	"foco/internal/provider"
	"foco/internal/requests"
)

var assessmentRoles = []enums.AssessmentRole{
	enums.AssessmentRoleImplementation,
	enums.AssessmentRoleCaller,
	enums.AssessmentRoleTest,
	enums.AssessmentRoleFixture,
	enums.AssessmentRoleHelper,
}

type assessment struct {
	Labels   []enums.AssessmentRole
	Priority float64
}

func assessFile(ctx context.Context, evaluator Evaluator, query string, entry candidate, preview *requests.FilePreview) (assessment, *provider.Failure) {
	value := requests.FilePreview{}
	if preview != nil {
		value = *preview
	}
	request := requests.FileAssessmentRequest(query, entry.Path, value)
	scores, err := evaluator.Evaluate(ctx, request, provider.EvalOptions{})
	if err != nil {
		return assessment{}, asFailure(err)
	}
	result := assessment{Priority: scores["priority"]}
	for _, role := range assessmentRoles {
		if scores[role.String()] > 0.5 {
			result.Labels = append(result.Labels, role)
		}
	}
	return result, nil
}
