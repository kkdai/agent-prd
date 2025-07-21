package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/google/go-github/v58/github"
	"google.golang.org/api/option"
)

const (
	LabelToTrigger         = "NEED_PRD"
	LabelToTriggerSubTask  = "NEED_SUB_TASK"
	PRDIdentifier          = "### PRD (Product Requirements Document)"
)

var (
	githubToken         = os.Getenv("GITHUB_TOKEN")
	googleAPIKey        = os.Getenv("GOOGLE_API_KEY")
	githubWebhookSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")
)

func main() {
	if githubToken == "" || googleAPIKey == "" || githubWebhookSecret == "" {
		log.Fatal("Missing required environment variables: GITHUB_TOKEN, GOOGLE_API_KEY, GITHUB_WEBHOOK_SECRET")
	}

	http.HandleFunc("/webhook", handleWebhook)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	log.Println("Received a webhook event")
	payload, err := github.ValidatePayload(r, []byte(githubWebhookSecret))
	if err != nil {
		log.Printf("Error validating payload: %v", err)
		http.Error(w, "Invalid payload", http.StatusUnauthorized)
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		log.Printf("Error parsing webhook: %v", err)
		http.Error(w, "Error parsing webhook", http.StatusBadRequest)
		return
	}

	switch event := event.(type) {
	case *github.IssuesEvent:
		log.Printf("Received issue event for action: %s", event.GetAction())
		issue := event.GetIssue()
		repo := event.GetRepo()
		if event.GetAction() == "labeled" {
			if hasLabel(issue.Labels, LabelToTrigger) {
				log.Printf("Issue #%d labeled with '%s', starting PRD generation.", issue.GetNumber(), LabelToTrigger)
				go processIssue(issue, repo)
			} else if hasLabel(issue.Labels, LabelToTriggerSubTask) {
				log.Printf("Issue #%d labeled with '%s', starting sub-task generation.", issue.GetNumber(), LabelToTriggerSubTask)
				go processSubTasks(issue, repo)
			}
		} else {
			log.Printf("Issue event for issue #%d did not meet processing criteria.", issue.GetNumber())
		}
	default:
		log.Printf("Ignoring webhook event type: %T", event)
	}

	w.WriteHeader(http.StatusOK)
}

func hasLabel(labels []*github.Label, labelName string) bool {
	for _, label := range labels {
		if label.GetName() == labelName {
			return true
		}
	}
	return false
}

func processIssue(issue *github.Issue, repo *github.Repository) {
	ctx := context.Background()
	client := github.NewClient(nil).WithAuthToken(githubToken)

	repoOwner := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	issueNumber := issue.GetNumber()

	log.Printf("Processing issue #%d in %s/%s", issueNumber, repoOwner, repoName)

	// Get README content
	readme, _, _, err := client.Repositories.GetContents(ctx, repoOwner, repoName, "README.md", nil)
	if err != nil {
		log.Printf("Error getting README for %s/%s: %v", repoOwner, repoName, err)
		return
	}
	readmeContent, err := readme.GetContent()
	if err != nil {
		log.Printf("Error decoding README content for %s/%s: %v", repoOwner, repoName, err)
		return
	}
	log.Printf("Successfully fetched README for %s/%s", repoOwner, repoName)

	// Generate PRD
	prdContent, err := generatePRD(issue.GetTitle(), issue.GetBody(), readmeContent)
	if err != nil {
		log.Printf("Error generating PRD for issue #%d: %v", issueNumber, err)
		return
	}
	log.Printf("Successfully generated PRD for issue #%d", issueNumber)

	// Post comment to issue
	comment := &github.IssueComment{
		Body: &prdContent,
	}
	log.Printf("Attempting to create comment on issue #%d", issueNumber)
	_, _, err = client.Issues.CreateComment(ctx, repoOwner, repoName, issue.GetNumber(), comment)
	if err != nil {
		log.Printf("Error creating comment on issue #%d: %v", issueNumber, err)
	} else {
		log.Printf("Successfully created PRD comment on issue #%d", issueNumber)
	}
}

func processSubTasks(issue *github.Issue, repo *github.Repository) {
	ctx := context.Background()
	client := github.NewClient(nil).WithAuthToken(githubToken)

	repoOwner := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	issueNumber := issue.GetNumber()

	log.Printf("Processing sub-tasks for issue #%d in %s/%s", issueNumber, repoOwner, repoName)

	// 1. Fetch comments to find the PRD
	comments, _, err := client.Issues.ListComments(ctx, repoOwner, repoName, issueNumber, nil)
	if err != nil {
		log.Printf("Error fetching comments for issue #%d: %v", issueNumber, err)
		return
	}

	var prdContent string
	for i := len(comments) - 1; i >= 0; i-- {
		comment := comments[i]
		if strings.Contains(comment.GetBody(), PRDIdentifier) {
			log.Printf("Found PRD comment #%d for issue #%d", comment.GetID(), issueNumber)
			prdContent = comment.GetBody()
			break
		}
	}

	if prdContent == "" {
		log.Printf("No PRD comment found for issue #%d. Aborting sub-task generation.", issueNumber)
		return
	}

	// 2. Generate Sub-tasks from PRD
	subTasks, err := generateSubTasks(prdContent)
	if err != nil {
		log.Printf("Error generating sub-tasks for issue #%d: %v", issueNumber, err)
		return
	}
	log.Printf("Successfully generated sub-tasks for issue #%d", issueNumber)

	// 3. Post the sub-tasks as a new comment
	comment := &github.IssueComment{
		Body: &subTasks,
	}
	log.Printf("Attempting to create sub-task comment on issue #%d", issueNumber)
	_, _, err = client.Issues.CreateComment(ctx, repoOwner, repoName, issueNumber, comment)
	if err != nil {
		log.Printf("Error creating sub-task comment on issue #%d: %v", issueNumber, err)
	} else {
		log.Printf("Successfully created sub-task comment on issue #%d", issueNumber)
	}
}

func generateSubTasks(prdContent string) (string, error) {
	ctx := context.Background()
	log.Println("Generating sub-tasks from PRD...")
	client, err := genai.NewClient(ctx, option.WithAPIKey(googleAPIKey))
	if err != nil {
		return "", err
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.0-flash")

	prompt := fmt.Sprintf(
		"As an expert project manager, break down the following Product Requirements Document (PRD) into a series of actionable sub-tasks for the development team. Each sub-task should be a single, distinct piece of work.\n\n"+
			"Format the output as a GitHub-flavored Markdown checklist. Each item should clearly state the main function to be completed.\n\n"+
			"**Example:**\n"+
			"- [ ] Set up the initial project structure and CI/CD pipeline.\n"+
			"- [ ] Develop the user authentication module.\n\n"+
			"**Here is the PRD:**\n%s",
		prdContent,
	)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("failed to generate sub-tasks: %w", err)
	}
	subTasks := extractText(resp)

	finalComment := fmt.Sprintf("### Generated Sub-tasks\n\n%s", subTasks)
	return finalComment, nil
}

func generatePRD(title, body, readme string) (string, error) {
	ctx := context.Background()
	log.Println("Generating PRD...")
	client, err := genai.NewClient(ctx, option.WithAPIKey(googleAPIKey))
	if err != nil {
		return "", err
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.0-flash")

	// 1. Generate English PRD
	log.Println("Generating English PRD...")
	promptEn := fmt.Sprintf(
		"As a professional Product Manager, create a Product Requirements Document (PRD) based on the following GitHub issue and repository README. The PRD should be in English.\n\n"+
			"**GitHub Issue Title:**\n%s\n\n"+
			"**GitHub Issue Body:**\n%s\n\n"+
			"**Repository README:**\n%s\n\n"+
			"**PRD Structure:**\n"+
			"1.  **Background:** (Briefly describe the context and problem)\n"+
			"2.  **Goals:** (What are the primary objectives?)\n"+
			"3.  **User Stories:** (As a [user type], I want [an action] so that [a benefit])\n"+
			"4.  **Requirements:** (Detailed functional and non-functional requirements)\n"+
			"5.  **Success Metrics:** (How will we measure success?)\n",
		title, body, readme,
	)

	respEn, err := model.GenerateContent(ctx, genai.Text(promptEn))
	if err != nil {
		return "", fmt.Errorf("failed to generate English PRD: %w", err)
	}
	englishPRD := extractText(respEn)
	log.Println("Successfully generated English PRD.")

	// 2. Detect language and generate translated PRD
	log.Println("Detecting language...")
	languageDetectionPrompt := fmt.Sprintf("Detect the primary language of the following text. Respond with the language name only (e.g., 'Traditional Chinese', 'Japanese').\n\nText:\n%s", body)
	respLang, err := model.GenerateContent(ctx, genai.Text(languageDetectionPrompt))
	if err != nil {
		log.Printf("Could not detect language, defaulting to original issue language for translation prompt: %v", err)
	}
	detectedLanguage := "the original language of the issue"
	if err == nil {
		detectedLanguage = extractText(respLang)
	}
	log.Printf("Detected language: %s", detectedLanguage)

	log.Printf("Generating translated PRD in %s...", detectedLanguage)
	promptTranslate := fmt.Sprintf(
		"Translate the following English PRD into %s. Maintain the original formatting and structure.\n\n**English PRD:**\n%s",
		detectedLanguage, englishPRD,
	)

	respTranslated, err := model.GenerateContent(ctx, genai.Text(promptTranslate))
	if err != nil {
		return "", fmt.Errorf("failed to generate translated PRD: %w", err)
	}
	translatedPRD := extractText(respTranslated)
	log.Printf("Successfully generated translated PRD in %s.", detectedLanguage)

	// Combine PRDs
	finalComment := fmt.Sprintf(
		"%s\n\n---\n\n%s\n\n---\n\n### PRD (%s)\n\n%s",
		PRDIdentifier,
		englishPRD,
		strings.TrimSpace(detectedLanguage),
		translatedPRD,
	)

	return finalComment, nil
}

func extractText(resp *genai.GenerateContentResponse) string {
	var b strings.Builder
	if resp != nil && resp.Candidates != nil {
		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if txt, ok := part.(genai.Text); ok {
						b.WriteString(string(txt))
					}
				}
			}
		}
	}
	return b.String()
}
