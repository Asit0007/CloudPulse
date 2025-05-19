// main.go: Go backend for CloudPulse dashboard
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"
)

func main() {
	// Validate GITHUB_TOKEN
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Fatal("Error: GITHUB_TOKEN environment variable is required")
	}

	// GitHub client setup
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	githubClient := github.NewClient(tc)

	// AWS client setup
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		log.Fatalf("Error: Unable to load AWS SDK config: %v", err)
	}
	awsClient := costexplorer.NewFromConfig(cfg)

	// Check if frontend directory exists
	if _, err := os.Stat("frontend"); os.IsNotExist(err) {
		log.Println("Warning: 'frontend' directory not found. Creating a default index.html...")
		if err := os.MkdirAll("frontend", 0755); err != nil {
			log.Fatalf("Error: Failed to create frontend directory: %v", err)
		}
		if err := os.WriteFile("frontend/index.html", []byte("<h1>Welcome to CloudPulse</h1><p>Go to /api/costs for AWS cost data or /api/github for GitHub data.</p>"), 0644); err != nil {
			log.Fatalf("Error: Failed to create default index.html: %v", err)
		}
	}

	// AWS Cost Explorer handler
	http.HandleFunc("/api/costs", func(w http.ResponseWriter, r *http.Request) {
		// Fetch cost data for the last 30 days
		end := time.Now()
		start := end.AddDate(0, 0, -30)

		// Format dates as strings and convert to *string
		startStr := start.Format("2006-01-02")
		endStr := end.Format("2006-01-02")
		log.Printf("Fetching AWS costs from %s to %s", startStr, endStr)

		input := &costexplorer.GetCostAndUsageInput{
			TimePeriod: &types.DateInterval{
				Start: aws.String(startStr),
				End:   aws.String(endStr),
			},
			Granularity: types.GranularityMonthly,
			Metrics:     []string{"UnblendedCost"},
			GroupBy: []types.GroupDefinition{
				{
					Type: types.GroupDefinitionTypeDimension,
					Key:  aws.String("SERVICE"),
				},
			},
		}

		result, err := awsClient.GetCostAndUsage(ctx, input)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching AWS cost data: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			http.Error(w, fmt.Sprintf("Error encoding response: %v", err), http.StatusInternalServerError)
		}
	})

	// GitHub API handler
	http.HandleFunc("/api/github", func(w http.ResponseWriter, r *http.Request) {
		user, _, err := githubClient.Users.Get(ctx, "")
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching GitHub user: %v", err), http.StatusInternalServerError)
			return
		}

		response := map[string]string{
			"login": user.GetLogin(),
			"name":  user.GetName(),
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, fmt.Sprintf("Error encoding response: %v", err), http.StatusInternalServerError)
		}
	})

	// Serve frontend
	http.Handle("/", http.FileServer(http.Dir("frontend")))

	// Start server
	port := ":8080"
	log.Printf("Server starting on %s", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Error: Server failed to start: %v. Is port %s in use?", err, port)
	}
}
