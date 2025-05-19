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

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"
)

// UsageData represents AWS usage statistics for the dashboard.
type UsageData struct {
	FreeTierLimit float64 `json:"freeTierLimit"`
	CurrentUsage  float64 `json:"currentUsage"`
}

// Contributor represents a GitHub contributor by login name.
type Contributor struct {
	Login string `json:"login"`
}

// getAWSUsage fetches the monthly AWS usage cost using the Cost Explorer API.
func getAWSUsage() (UsageData, error) {
	// Load AWS config with default credentials and region
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-east-1"))
	if err != nil {
		return UsageData{}, fmt.Errorf("unable to load AWS config: %w", err)
	}

	client := costexplorer.NewFromConfig(cfg)
	end := time.Now()
	start := end.AddDate(0, -1, 0) // Last month's start date

	// Prepare input for the Cost Explorer API
	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &types.DateInterval{
			Start: start.Format("2006-01-02"),
			End:   end.Format("2006-01-02"),
		},
		Granularity: types.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
	}

	// Fetch usage data
	result, err := client.GetCostAndUsage(context.TODO(), input)
	if err != nil {
		return UsageData{}, fmt.Errorf("failed to get AWS usage: %w", err)
	}

	// Parse total cost from result
	var totalCost float64
	for _, group := range result.ResultsByTime {
		for _, metric := range group.Total {
			// Convert the string value to float64
			cost, err := parseCost(metric.Amount)
			if err != nil {
				return UsageData{}, err
			}
			totalCost += cost
		}
	}

	// Return usage data (adjust free tier limit as needed)
	return UsageData{
		FreeTierLimit: 100.0, // Example: $100/month
		CurrentUsage:  totalCost,
	}, nil
}

// parseCost safely parses a cost string to float64.
func parseCost(amount *string) (float64, error) {
	if amount == nil {
		return 0, fmt.Errorf("nil cost amount")
	}
	var val float64
	_, err := fmt.Sscanf(*amount, "%f", &val)
	if err != nil {
		return 0, fmt.Errorf("invalid cost amount: %w", err)
	}
	return val, nil
}

// getGitHubContributors fetches contributors from the specified GitHub repo.
func getGitHubContributors() ([]Contributor, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN not set")
	}

	// Authenticate using personal access token
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// Replace with actual GitHub username and repo
	const owner = "your-username"
	const repo = "cloudpulse"

	// Get contributors list
	contributors, _, err := client.Repositories.ListContributors(ctx, owner, repo, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub contributors: %w", err)
	}

	// Map contributor login names
	var result []Contributor
	for _, c := range contributors {
		if c.Login != nil {
			result = append(result, Contributor{Login: *c.Login})
		}
	}
	return result, nil
}

func main() {
	// Route: AWS Usage API
	http.HandleFunc("/api/usage", func(w http.ResponseWriter, r *http.Request) {
		data, err := getAWSUsage()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, data)
	})

	// Route: GitHub Contributors API
	http.HandleFunc("/api/contributors", func(w http.ResponseWriter, r *http.Request) {
		contributors, err := getGitHubContributors()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, contributors)
	})

	// Serve frontend files from the relative path
	http.Handle("/", http.FileServer(http.Dir("../frontend")))

	log.Println("Starting server on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// writeJSON encodes and writes data as JSON with proper headers.
func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
	}
}
