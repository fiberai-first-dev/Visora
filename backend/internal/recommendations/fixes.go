package recommendations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sashabaranov/go-openai"
	"visora-backend/internal/db"
	"visora-backend/internal/llm"
)

type GeneratedFix struct {
	Patch string `json:"patch"`
	Note  string `json:"note"`
}

// GenerateFixes attaches AI patches onto open recommendations that still lack Data.
// No template fallback — missing API or empty model output fails the job.
func GenerateFixes(projectID uint) error {
	var recs []db.Recommendation
	if err := db.DB.Where("project_id = ? AND status = 'open'", projectID).Find(&recs).Error; err != nil {
		return err
	}

	need := make([]db.Recommendation, 0, len(recs))
	for _, rec := range recs {
		if rec.Data == "" || rec.Data == "{}" || rec.Data == `{"origin":"ai"}` {
			need = append(need, rec)
		}
	}
	if len(need) == 0 {
		return nil
	}

	var project db.Project
	_ = db.DB.First(&project, projectID).Error

	patched := 0
	var firstErr error
	for _, rec := range need {
		prompt := fmt.Sprintf(
			`Brand %s (%s). Generate a specific code/text patch for: %s. Details: %s. Action: %s.
Return JSON: {"patch":"...","note":"..."}`,
			project.Brand, project.Website, rec.Title, rec.Detail, rec.Action,
		)

		raw, err := llm.CompleteJSON(context.Background(), llm.GetModel(), []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "You are an expert SEO engineer. Write precise AI patches as JSON only."},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		}, 0.2)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		var fix GeneratedFix
		if json.Unmarshal([]byte(raw), &fix) != nil || fix.Patch == "" {
			continue
		}
		fixData, _ := json.Marshal(map[string]string{
			"patch":  fix.Patch,
			"note":   fix.Note,
			"origin": "ai",
		})
		rec.Data = string(fixData)
		db.DB.Save(&rec)
		patched++
	}

	if patched == 0 {
		if firstErr != nil {
			return fmt.Errorf("AI recommendation patches required: %w", firstErr)
		}
		return fmt.Errorf("AI recommendation patches required: empty output")
	}
	return nil
}
