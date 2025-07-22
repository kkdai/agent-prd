package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/generative-ai-go/genai"
	"github.com/google/go-github/v58/github"
	"google.golang.org/api/option"
)

const (
	CommandGeneratePRD     = "need_prd"
	CommandGenerateSubTask = "need_sub_task"
	PRDIdentifier          = "### PRD (Product Requirements Document)"
)

var (
	githubAppID         = os.Getenv("GITHUB_APP_ID")
	githubAppPrivateKey = os.Getenv("GITHUB_APP_PRIVATE_KEY")
	githubAppName       = os.Getenv("GITHUB_APP_NAME")
	googleAPIKey        = os.Getenv("GOOGLE_API_KEY")
	githubWebhookSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")
)

func main() {
	if githubAppID == "" || githubAppPrivateKey == "" || githubAppName == "" || googleAPIKey == "" || githubWebhookSecret == "" {
		log.Fatal("Missing required environment variables: GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY, GITHUB_APP_NAME, GOOGLE_API_KEY, GITHUB_WEBHOOK_SECRET")
	}

	http.HandleFunc("/webhook", handleWebhook)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func createGitHubClient(installationID int64) (*github.Client, error) {
	appID, err := strconv.ParseInt(githubAppID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid GITHUB_APP_ID: %w", err)
	}

	itr, err := ghinstallation.New(http.DefaultTransport, appID, installationID, []byte(githubAppPrivateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create installation transport: %w", err)
	}

	httpClient := &http.Client{Transport: itr}
	return github.NewClient(httpClient), nil
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
	log.Printf("Successfully parsed webhook event of type: %T", event)

	switch event := event.(type) {
	case *github.IssueCommentEvent:
		if event.GetAction() != "created" {
			log.Printf("Ignoring comment event action: %s", event.GetAction())
			return
		}

		commentBody := event.GetComment().GetBody()
		botMention := "@" + githubAppName

		// --- Enhanced Logging and Parsing Logic ---
		log.Printf("DEBUG: Raw comment body received: [%s]", commentBody)
		log.Printf("DEBUG: Checking for bot mention: [%s]", botMention)

		trimmedBody := strings.TrimSpace(commentBody)
		fields := strings.Fields(trimmedBody)

		if len(fields) == 0 || fields[0] != botMention {
			log.Printf("Comment does not start with the bot mention. Expected '%s' as the first word, but got '%s'. Ignoring.", botMention, fields)
			return
		}
		// --- End of Enhanced Logic ---

		issue := event.GetIssue()
		repo := event.GetRepo()
		installationID := event.GetInstallation().GetID()
		log.Printf("Bot was mentioned correctly in a comment on issue #%d", issue.GetNumber())

		client, err := createGitHubClient(installationID)
		if err != nil {
			log.Printf("Error creating GitHub client: %v", err)
			return
		}
		ctx := context.Background()

		// Check for commands in the rest of the fields
		var commandFound bool
		for _, field := range fields {
			if field == CommandGeneratePRD {
				log.Printf("Found command '%s' for issue #%d", CommandGeneratePRD, issue.GetNumber())
				go processIssue(ctx, client, issue, repo)
				commandFound = true
				break
			}
			if field == CommandGenerateSubTask {
				log.Printf("Found command '%s' for issue #%d", CommandGenerateSubTask, issue.GetNumber())
				go processSubTasks(ctx, client, issue, repo)
				commandFound = true
				break
			}
		}

		if !commandFound {
			log.Printf("Bot was mentioned on issue #%d, but no valid command was found.", issue.GetNumber())
		}

	default:
		log.Printf("Ignoring webhook event type: %T", event)
	}

	w.WriteHeader(http.StatusOK)
}

func findPRDComment(ctx context.Context, client *github.Client, repoOwner, repoName string, issueNumber int) (*github.IssueComment, error) {
	comments, _, err := client.Issues.ListComments(ctx, repoOwner, repoName, issueNumber, nil)
	if err != nil {
		return nil, fmt.Errorf("error fetching comments for issue #%d: %w", issueNumber, err)
	}

	for i := len(comments) - 1; i >= 0; i-- {
		comment := comments[i]
		if strings.Contains(comment.GetBody(), PRDIdentifier) {
			log.Printf("Found PRD comment #%d for issue #%d", comment.GetID(), issueNumber)
			return comment, nil
		}
	}
	return nil, nil // No PRD found
}

func processIssue(ctx context.Context, client *github.Client, issue *github.Issue, repo *github.Repository) {
	repoOwner := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	issueNumber := issue.GetNumber()

	log.Printf("Processing '%s' for issue #%d in %s/%s", CommandGeneratePRD, issueNumber, repoOwner, repoName)

	existingPRD, err := findPRDComment(ctx, client, repoOwner, repoName, issueNumber)
	if err != nil {
		log.Printf("Error checking for existing PRD on issue #%d: %v", issueNumber, err)
		return
	}
	if existingPRD != nil {
		log.Printf("PRD already exists for issue #%d. Skipping generation.", issueNumber)
		return
	}

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

	prdContent, err := generatePRD(issue.GetTitle(), issue.GetBody(), readmeContent)
	if err != nil {
		log.Printf("Error generating PRD for issue #%d: %v", issueNumber, err)
		return
	}
	log.Printf("Successfully generated PRD for issue #%d", issueNumber)

	comment := &github.IssueComment{
		Body: &prdContent,
	}
	log.Printf("Attempting to create PRD comment on issue #%d", issueNumber)
	_, _, err = client.Issues.CreateComment(ctx, repoOwner, repoName, issue.GetNumber(), comment)
	if err != nil {
		log.Printf("Error creating PRD comment on issue #%d: %v", issueNumber, err)
	} else {
		log.Printf("Successfully created PRD comment on issue #%d", issueNumber)
	}
}

func processSubTasks(ctx context.Context, client *github.Client, issue *github.Issue, repo *github.Repository) {
	repoOwner := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	issueNumber := issue.GetNumber()

	log.Printf("Processing '%s' for issue #%d in %s/%s", CommandGenerateSubTask, issueNumber, repoOwner, repoName)

	prdComment, err := findPRDComment(ctx, client, repoOwner, repoName, issueNumber)
	if err != nil {
		log.Printf("Error finding PRD comment for issue #%d: %v", issueNumber, err)
		return
	}
	if prdComment == nil {
		log.Printf("No PRD comment found for issue #%d. Aborting sub-task generation.", issueNumber)
		noPrdMessage := fmt.Sprintf("I couldn't find a PRD to generate sub-tasks from. Please run `@%s %s` first.", githubAppName, CommandGeneratePRD)
		comment := &github.IssueComment{Body: &noPrdMessage}
		_, _, _ = client.Issues.CreateComment(ctx, repoOwner, repoName, issueNumber, comment)
		return
	}

	subTasks, err := generateSubTasks(prdComment.GetBody())
	if err != nil {
		log.Printf("Error generating sub-tasks for issue #%d: %v", issueNumber, err)
		return
	}
	log.Printf("Successfully generated sub-tasks for issue #%d", issueNumber)

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

	finalComment := fmt.Sprintf("### Generated Sub-tasks\n\nBased on the PRD, here are the suggested sub-tasks:\n\n%s", subTasks)
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

	promptTranslate := fmt.Sprintf(
		"Translate the following English PRD into %s. Maintain the original formatting and structure.\n\n**English PRD:**\n%s",
		detectedLanguage, englishPRD,
	)

	respTranslated, err := model.GenerateContent(ctx, genai.Text(promptTranslate))
	if err != nil {
		log.Printf("Failed to generate translated PRD, falling back to English only: %v", err)
		return fmt.Sprintf(
			"%s\n\n---\n\n%s",
			PRDIdentifier,
			englishPRD,
		), nil
	}
	translatedPRD := extractText(respTranslated)
	log.Printf("Successfully generated translated PRD in %s.", detectedLanguage)

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
