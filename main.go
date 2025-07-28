package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/go-github/v55/github"
	"golang.org/x/oauth2"
)

func commitMultipleFiles(client *github.Client, owner, repo, branch string, files map[string]string, commitMessage string) error {
	ctx := context.Background()

	// 1. Get reference to the branch
	ref, _, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("GetRef: %w", err)
	}

	// 2. Skip getting parent commit if you're replacing everything,
	//    but we still need the tree base (we'll use empty tree).
	baseTreeSHA := "" // empty base

	// 3. Parallel blob creation
	type blobResult struct {
		path string
		sha  string
		err  error
	}

	results := make(chan blobResult, len(files))
	for path, content := range files {
		go func(p, c string) {
			blob, _, err := client.Git.CreateBlob(ctx, owner, repo, &github.Blob{
				Content:  github.String(c),
				Encoding: github.String("utf-8"),
			})
			if err != nil {
				results <- blobResult{p, "", err}
				return
			}
			results <- blobResult{p, blob.GetSHA(), nil}
		}(path, content)
	}

	// 4. Collect blobs into tree entries
	var entries []*github.TreeEntry
	for i := 0; i < len(files); i++ {
		r := <-results
		if r.err != nil {
			return fmt.Errorf("blob error for %s: %w", r.path, r.err)
		}
		entries = append(entries, &github.TreeEntry{
			Path: github.String(r.path),
			Mode: github.String("100644"),
			Type: github.String("blob"),
			SHA:  github.String(r.sha),
		})
	}

	// 5. Create new tree
	tree, _, err := client.Git.CreateTree(ctx, owner, repo, baseTreeSHA, entries)
	if err != nil {
		return fmt.Errorf("CreateTree: %w", err)
	}

	// 6. Create a commit (no parent if replacing everything)
	newCommit := &github.Commit{
		Message: github.String(commitMessage),
		Tree:    tree,
	}

	commit, _, err := client.Git.CreateCommit(ctx, owner, repo, newCommit)
	if err != nil {
		return fmt.Errorf("CreateCommit: %w", err)
	}

	// 7. Update ref to point to new commit
	ref.Object.SHA = commit.SHA
	_, _, err = client.Git.UpdateRef(ctx, owner, repo, ref, true) // force=true replaces history
	if err != nil {
		return fmt.Errorf("UpdateRef: %w", err)
	}

	fmt.Println("✅ Commit created:", commit.GetHTMLURL())
	return nil
}
func createRepo(client *github.Client, owner, repoName string) error {
	ctx := context.Background()

	// Check if the repository already exists
	_, resp, err := client.Repositories.Get(ctx, owner, repoName)
	if err == nil {
		log.Println("Repo already exists:", fmt.Sprintf("https://github.com/%s/%s", owner, repoName))
		return nil
	}
	if resp != nil && resp.StatusCode != 404 {
		return fmt.Errorf("Error checking if repo exists: %w", err)
	}

	// Repo doesn't exist, so create it
	repo := &github.Repository{
		Name:        github.String(repoName),
		Private:     github.Bool(false),
		AutoInit:    github.Bool(true),                            // This initializes repo with a README
		Description: github.String("Auto-created with Go script"), // Message of the readme file of the repo
		License: &github.License{
			Key: github.String("mit"), // Change to desired license key
		},
	}

	createdRepo, _, err := client.Repositories.Create(ctx, "", repo)
	if err != nil {
		return fmt.Errorf("Error creating repo: %w", err)
	}

	log.Println("Repo created:", createdRepo.GetHTMLURL())
	return nil
}
func deleteRepo(client *github.Client, owner, repoName string) error {
	ctx := context.Background()

	_, err := client.Repositories.Delete(ctx, owner, repoName)
	if err != nil {
		return fmt.Errorf("failed to delete repo %s/%s: %w", owner, repoName, err)
	}

	fmt.Printf("✅ Deleted repo: https://github.com/%s/%s\n", owner, repoName)
	return nil
}
func main() {

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN is not set in the environment")
	}

	owner := "Santosh-etailify" // change this
	repo := "gitapis-go-github" // change this
	branch := "main"            // change if needed
	commitMessage := "AutoInitialized main branch with a default readme file"
	// change if needed

	localFiles := []string{
		"main.go", "temp/temp.go", // Add your local file paths here
	}

	files := make(map[string]string)

	for _, localPath := range localFiles {
		content, err := os.ReadFile(localPath)
		if err != nil {
			log.Fatalf("Failed to read %s: %v", localPath, err)
		}

		// Use forward slashes even on Windows for GitHub paths
		//repoPath := filepath.ToSlash(localPath)

		files[localPath] = string(content)
	}

	//GitHub Client
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	//Run required functions
	err := createRepo(client, owner, repo)
	if err != nil {
		log.Fatalf("Failed to create repo: %v", err)
	}
	start := time.Now()

	err = commitMultipleFiles(client, owner, repo, branch, files, commitMessage)
	if err != nil {
		log.Fatalf("Failed to upsert files: %v", err)
	}
	duration := time.Since(start)
	fmt.Printf("⏱️  upsertMultipleFilesSafe took: %v\n", duration)
	//Print Summary
	// fmt.Println("File Update Summary:")
	// for file, status := range result {
	// 	fmt.Printf("  %s → %s\n", file, status)
	// }
}
