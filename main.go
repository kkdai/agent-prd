package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/generative-ai-go/genai"
	"github.com/google/go-github/v58/github"
	"google.golang.org/api/option"
)

// --- Constants and Configuration ---

const (
	CommandGeneratePRD      = "need_prd"
	CommandGenerateSubTask  = "need_sub_task"
	CommandImplementFeature = "implement_feature"
	PRDIdentifier           = "### PRD (Product Requirements Document)"
)

var (
	githubAppID         = os.Getenv("GITHUB_APP_ID")
	githubAppPrivateKey = os.Getenv("GITHUB_APP_PRIVATE_KEY")
	githubAppName       = strings.TrimSpace(os.Getenv("GITHUB_APP_NAME"))
	googleAPIKey        = os.Getenv("GOOGLE_API_KEY")
	githubWebhookSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")
)

// --- Bot Structure and Command Handling ---

// Bot holds the application's configuration and command registry.
type Bot struct {
	appName  string
	commands map[string]commandHandler
}

// commandHandler defines the function signature for a bot command.
type commandHandler func(ctx context.Context, client *github.Client, issue *github.Issue, repo *github.Repository, installationID int64)

// NewBot creates and initializes a new Bot instance.
func NewBot(appName string) *Bot {
	bot := &Bot{
		appName:  appName,
		commands: make(map[string]commandHandler),
	}
	bot.registerCommands()
	return bot
}

// registerCommands maps command strings to their handler functions.
func (b *Bot) registerCommands() {
	b.commands[CommandGeneratePRD] = b.processIssuePRD
	b.commands[CommandGenerateSubTask] = b.processIssueSubTasks
	b.commands[CommandImplementFeature] = b.processImplementFeature
}

// --- Main Application ---

func main() {
	if githubAppID == "" || githubAppPrivateKey == "" || githubAppName == "" || googleAPIKey == "" || githubWebhookSecret == "" {
		log.Fatal("Missing required environment variables: GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY, GITHUB_APP_NAME, GOOGLE_API_KEY, GITHUB_WEBHOOK_SECRET")
	}

	bot := NewBot(githubAppName)
	http.HandleFunc("/webhook", bot.handleWebhook)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// --- Webhook and Authentication ---

func (b *Bot) handleWebhook(w http.ResponseWriter, r *http.Request) {
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
	var installationID int64
	var issue *github.Issue
	var repo *github.Repository
	var action string
	var commentBody string

	switch e := event.(type) {
	case *github.IssuesEvent:
		installationID = e.GetInstallation().GetID()
		issue = e.GetIssue()
		repo = e.GetRepo()
		action = e.GetAction()
		if action == "opened" {
			log.Printf("New issue opened #%d. Triggering PRD generation.", issue.GetNumber())
			client, err := createGitHubClient(installationID)
			if err != nil {
				log.Printf("Error creating GitHub client for new issue: %v", err)
				return
			}
			go b.processIssuePRD(context.Background(), client, issue, repo, installationID)
		}
		return // Return after handling
	case *github.IssueCommentEvent:
		installationID = e.GetInstallation().GetID()
		issue = e.GetIssue()
		repo = e.GetRepo()
		action = e.GetAction()
		commentBody = e.GetComment().GetBody()
	default:
		log.Printf("Ignoring event of type %T", event)
		w.WriteHeader(http.StatusOK)
		return
	}

	if action != "created" {
		log.Printf("Ignoring non-created issue comment event.")
		w.WriteHeader(http.StatusOK)
		return
	}

	command, mentioned := b.parseComment(commentBody)
	if !mentioned {
		log.Printf("Bot was not mentioned correctly in comment.")
		w.WriteHeader(http.StatusOK)
		return
	}

	handler, exists := b.commands[command]
	if !exists {
		log.Printf("Bot was mentioned, but command '%s' is not recognized.", command)
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("Recognized command '%s' on issue #%d. Dispatching handler.", issue.GetNumber(), command)
	client, err := createGitHubClient(installationID)
	if err != nil {
		log.Printf("Error creating GitHub client for comment: %v", err)
		http.Error(w, "Failed to create client", http.StatusInternalServerError)
		return
	}

	go handler(context.Background(), client, issue, repo, installationID)
	w.WriteHeader(http.StatusOK)
}

func createGitHubClient(installationID int64) (*github.Client, error) {
	appID, err := strconv.ParseInt(githubAppID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid GITHUB_APP_ID: %w", err)
	}
	privateKeyBytes, err := base64.StdEncoding.DecodeString(githubAppPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 private key: %w", err)
	}
	itr, err := ghinstallation.New(http.DefaultTransport, appID, installationID, privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create installation transport: %w", err)
	}
	return github.NewClient(&http.Client{Transport: itr}), nil
}

func getInstallationToken(ctx context.Context, installationID int64) (string, error) {
	appID, err := strconv.ParseInt(githubAppID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid GITHUB_APP_ID: %w", err)
	}
	privateKeyBytes, err := base64.StdEncoding.DecodeString(githubAppPrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 private key: %w", err)
	}
	itr, err := ghinstallation.New(http.DefaultTransport, appID, installationID, privateKeyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create installation transport: %w", err)
	}
	token, err := itr.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get installation token: %w", err)
	}
	return token, nil
}

// --- Command Implementations ---

func (b *Bot) processIssuePRD(ctx context.Context, client *github.Client, issue *github.Issue, repo *github.Repository, _ int64) {
	repoOwner, repoName, issueNum := repo.GetOwner().GetLogin(), repo.GetName(), issue.GetNumber()
	log.Printf("Processing '%s' for issue #%d in %s/%s", CommandGeneratePRD, issueNum, repoOwner, repoName)

	if prd, _ := findPRDComment(ctx, client, repoOwner, repoName, issueNum); prd != nil {
		log.Printf("PRD already exists for issue #%d. Skipping generation.", issueNum)
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

	prdContent, err := generatePRD(issue.GetTitle(), issue.GetBody(), readmeContent)
	if err != nil {
		log.Printf("Error generating PRD for issue #%d: %v", issueNum, err)
		return
	}

	b.postComment(ctx, client, repoOwner, repoName, issueNum, prdContent)
}

func (b *Bot) processIssueSubTasks(ctx context.Context, client *github.Client, issue *github.Issue, repo *github.Repository, _ int64) {
	repoOwner, repoName, issueNum := repo.GetOwner().GetLogin(), repo.GetName(), issue.GetNumber()
	log.Printf("Processing '%s' for issue #%d in %s/%s", CommandGenerateSubTask, issueNum, repoOwner, repoName)

	prdComment, err := findPRDComment(ctx, client, repoOwner, repoName, issueNum)
	if err != nil || prdComment == nil {
		log.Printf("No PRD comment found for issue #%d. Aborting sub-task generation.", issueNum)
		noPrdMessage := fmt.Sprintf("I couldn't find a PRD to generate sub-tasks from. Please run `@%s %s` first.", b.appName, CommandGeneratePRD)
		b.postComment(ctx, client, repoOwner, repoName, issueNum, noPrdMessage)
		return
	}

	subTasks, err := generateSubTasks(prdComment.GetBody())
	if err != nil {
		log.Printf("Error generating sub-tasks for issue #%d: %v", issueNum, err)
		return
	}

	b.postComment(ctx, client, repoOwner, repoName, issueNum, subTasks)
}

func (b *Bot) processImplementFeature(ctx context.Context, client *github.Client, issue *github.Issue, repo *github.Repository, installationID int64) {
	repoOwner, repoName, issueNum := repo.GetOwner().GetLogin(), repo.GetName(), issue.GetNumber()
	log.Printf("Processing '%s' for issue #%d in %s/%s", CommandImplementFeature, issueNum, repoOwner, repoName)

	// Helper function for posting failure comments
	fail := func(reason string, err error) {
		log.Printf("Operation failed for issue #%d: %s: %v", issueNum, reason, err)
		errMsg := fmt.Sprintf("I failed to implement the feature for issue #%d. **Reason:** %s.", issueNum, reason)
		b.postComment(ctx, client, repoOwner, repoName, issueNum, errMsg)
	}

	filesToModify := parseFilePathsFromIssue(issue.GetBody())
	if len(filesToModify) == 0 {
		fail("No files to modify. Please specify the files in the issue body using the format `Files: file1.go, path/to/file2.go`", nil)
		return
	}

	b.postComment(ctx, client, repoOwner, repoName, issueNum, fmt.Sprintf("Alright, I'm on it! I will try to implement the feature for issue #%d. Give me a few minutes...", issueNum))

	tempDir, err := os.MkdirTemp("", fmt.Sprintf("repo-%d-*", issueNum))
	if err != nil {
		fail("Could not create temporary directory", err)
		return
	}
	defer os.RemoveAll(tempDir)
	log.Printf("Created temporary directory: %s", tempDir)

	token, err := getInstallationToken(ctx, installationID)
	if err != nil {
		fail("Could not get installation token", err)
		return
	}

	cloneURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, repoOwner, repoName)
	if _, err := runCommand(tempDir, "git", "clone", cloneURL, "."); err != nil {
		fail("Could not clone repository", err)
		return
	}

	branchName := fmt.Sprintf("feature/issue-%d-%d", issueNum, time.Now().Unix())
	if _, err := runCommand(tempDir, "git", "checkout", "-b", branchName); err != nil {
		fail("Could not create new branch", err)
		return
	}

	prompt := fmt.Sprintf("As a senior Go developer, please modify the code to implement the feature described in the following GitHub issue.\n\n**Issue Title:** %s\n\n**Issue Body:**\n%s\n\nYour response should only be the modified code, without any additional explanation.", issue.GetTitle(), issue.GetBody())
	geminiArgs := []string{prompt, "-y", "-a"}
	geminiArgs = append(geminiArgs, filesToModify...)

	if _, err := runCommand(tempDir, "gemini", geminiArgs...); err != nil {
		fail("Gemini CLI failed to modify the files", err)
		return
	}

	if _, err := runCommand(tempDir, "git", "config", "user.name", b.appName); err != nil {
		fail("Could not set git user name", err)
		return
	}
	if _, err := runCommand(tempDir, "git", "config", "user.email", fmt.Sprintf("%s@users.noreply.github.com", b.appName)); err != nil {
		fail("Could not set git user email", err)
		return
	}

	if _, err := runCommand(tempDir, "git", "add", "."); err != nil {
		fail("Could not add files to git", err)
		return
	}

	commitMsg := fmt.Sprintf("feat: Implement feature for #%d\n\nThis commit was automatically generated by the Gemini bot based on the issue.", issueNum)
	if _, err := runCommand(tempDir, "git", "commit", "-m", commitMsg); err != nil {
		fail("Could not commit changes", err)
		return
	}

	if _, err := runCommand(tempDir, "git", "push", "origin", branchName); err != nil {
		fail("Could not push changes to remote", err)
		return
	}

	prTitle := fmt.Sprintf("Implement Feature: %s", issue.GetTitle())
	prBody := fmt.Sprintf("This PR implements the feature requested in #%d. It was automatically generated by @%s.", issueNum, b.appName)
	newPR := &github.NewPullRequest{
		Title: &prTitle,
		Head:  &branchName,
		Base:  repo.DefaultBranch,
		Body:  &prBody,
	}

	pr, _, err := client.PullRequests.Create(ctx, repoOwner, repoName, newPR)
	if err != nil {
		fail("Could not create Pull Request", err)
		return
	}

	finalComment := fmt.Sprintf("I've created a Pull Request for issue #%d. You can review it here: %s", issueNum, pr.GetHTMLURL())
	b.postComment(ctx, client, repoOwner, repoName, issueNum, finalComment)
}

// --- Helper Functions ---

func runCommand(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	log.Printf("Executing command in %s: %s", dir, cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Command failed with error: %v\nOutput:\n%s", err, string(output))
		return string(output), err
	}
	log.Printf("Command executed successfully. Output:\n%s", string(output))
	return string(output), nil
}

func parseFilePathsFromIssue(body string) []string {
	var files []string
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Files:") {
			filesPart := strings.TrimSpace(strings.TrimPrefix(line, "Files:"))
			if filesPart != "" {
				for _, f := range strings.Split(filesPart, ",") {
					files = append(files, strings.TrimSpace(f))
				}
			}
			// Found the line, stop processing
			return files
		}
	}
	return files
}

func (b *Bot) parseComment(body string) (command string, mentioned bool) {
	botMention := "@" + b.appName
	trimmedBody := strings.TrimSpace(body)
	fields := strings.Fields(trimmedBody)

	if len(fields) < 2 || fields[0] != botMention {
		return "", false
	}

	return fields[1], true
}

func (b *Bot) postComment(ctx context.Context, client *github.Client, owner, repo string, issueNum int, body string) {
	comment := &github.IssueComment{Body: &body}
	log.Printf("Attempting to post comment to issue #%d", issueNum)
	_, _, err := client.Issues.CreateComment(ctx, owner, repo, issueNum, comment)
	if err != nil {
		log.Printf("Error creating comment on issue #%d: %v", issueNum, err)
	} else {
		log.Printf("Successfully created comment on issue #%d", issueNum)
	}
}

func findPRDComment(ctx context.Context, client *github.Client, repoOwner, repoName string, issueNumber int) (*github.IssueComment, error) {
	comments, _, err := client.Issues.ListComments(ctx, repoOwner, repoName, issueNumber, nil)
	if err != nil {
		return nil, fmt.Errorf("error fetching comments for issue #%d: %w", issueNumber, err)
	}
	for i := len(comments) - 1; i >= 0; i-- {
		if strings.Contains(comments[i].GetBody(), PRDIdentifier) {
			log.Printf("Found PRD comment #%d for issue #%d", comments[i].GetID(), issueNumber)
			return comments[i], nil
		}
	}
	return nil, nil // No PRD found
}

// --- AI Generation Functions (Unchanged) ---

func generateSubTasks(prdContent string) (string, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(googleAPIKey))
	if err != nil {
		return "", err
	}
	defer client.Close()
	model := client.GenerativeModel("gemini-1.5-flash")
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
	return fmt.Sprintf("### Generated Sub-tasks\n\nBased on the PRD, here are the suggested sub-tasks:\n\n%s", extractText(resp)), nil
}

func generatePRD(title, body, readme string) (string, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(googleAPIKey))
	if err != nil {
		return "", err
	}
	defer client.Close()
	model := client.GenerativeModel("gemini-1.5-flash")

	// Generate English PRD
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

	// Detect language and translate
	languageDetectionPrompt := fmt.Sprintf("Detect the primary language of the following text. Respond with the language name only (e.g., 'Traditional Chinese', 'Japanese').\n\nText:\n%s", body)
	respLang, err := model.GenerateContent(ctx, genai.Text(languageDetectionPrompt))
	detectedLanguage := "the original language of the issue"
	if err == nil {
		detectedLanguage = extractText(respLang)
	}

	promptTranslate := fmt.Sprintf("Translate the following English PRD into %s. Maintain the original formatting and structure.\n\n**English PRD:**\n%s", detectedLanguage, englishPRD)
	respTranslated, err := model.GenerateContent(ctx, genai.Text(promptTranslate))
	if err != nil {
		log.Printf("Failed to generate translated PRD, falling back to English only: %v", err)
		return fmt.Sprintf("%s\n\n---\n\n%s", PRDIdentifier, englishPRD), nil
	}
	translatedPRD := extractText(respTranslated)

	return fmt.Sprintf(
		"%s\n\n---\n\n%s\n\n---\n\n### PRD (%s)\n\n%s",
		PRDIdentifier, englishPRD, strings.TrimSpace(detectedLanguage), translatedPRD,
	), nil
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
