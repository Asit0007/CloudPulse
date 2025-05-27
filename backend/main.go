package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws" // <-- ADDED for SDK helpers (aws.String, aws.Int32)
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/google/go-github/v58/github" // Ensure this matches your go.mod
	vault "github.com/hashicorp/vault/api"
	"golang.org/x/oauth2"
)

// Global variables for clients - initialize once
var (
	cwClient     *cloudwatch.Client
	githubClient *github.Client
	vaultClient  *vault.Client
	instanceID   string // Store EC2 Instance ID
	githubOwner  string // GitHub Repo Owner
	githubRepo   string // GitHub Repo Name
)

// --- Vault Functions ---

// initVault initializes the Vault client.
func initVault() error {
	conf := vault.DefaultConfig() // Reads VAULT_ADDR from env (e.g., http://127.0.0.1:8200)

	var err error
	vaultClient, err = vault.NewClient(conf)
	if err != nil {
		return fmt.Errorf("failed to create vault client: %w", err)
	}

	token := os.Getenv("VAULT_TOKEN")
	if token == "" {
		return fmt.Errorf("VAULT_TOKEN environment variable not set")
	}
	vaultClient.SetToken(token)

	log.Println("Vault client initialized successfully.")
	return nil
}

// getSecret fetches a secret from Vault's KVv2 store.
func getSecret(secretPath, key string) (string, error) {
	if vaultClient == nil {
		return "", fmt.Errorf("vault client not initialized")
	}

	// For KVv2, the API path is 'mount/data/path'. We need to extract mount and path.
	parts := strings.SplitN(secretPath, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid secret path format '%s', expected 'mount/path'", secretPath)
	}
	mountPath := parts[0]
	pathWithinMount := parts[1]

	log.Printf("Fetching secret '%s' from Vault mount '%s' path '%s'\n", key, mountPath, pathWithinMount)
	secret, err := vaultClient.KVv2(mountPath).Get(context.Background(), pathWithinMount)
	if err != nil {
		return "", fmt.Errorf("failed to get secret from Vault (path: %s): %w", secretPath, err)
	}

	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("no data found at secret path '%s'", secretPath)
	}

	value, ok := secret.Data[key].(string)
	if !ok {
		return "", fmt.Errorf("secret key '%s' not found or not a string in path '%s'", key, secretPath)
	}

	log.Printf("Successfully fetched secret '%s' from Vault.", key)
	return value, nil
}

// --- AWS Functions ---

// initAWS initializes the AWS CloudWatch client and fetches the instance ID.
func initAWS() error {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	cwClient = cloudwatch.NewFromConfig(cfg)

	// Fetch instance ID using HTTP GET from metadata service (simpler & reliable)
	metadataURL := "http://169.254.169.254/latest/meta-data/instance-id"
	// Set a timeout for the HTTP request to avoid hangs if metadata service is not available
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(metadataURL)

	if err != nil {
		log.Printf("Could not fetch EC2 instance ID via HTTP: %v. Using override or placeholder.", err)
		instanceID = os.Getenv("EC2_INSTANCE_ID_OVERRIDE")
		if instanceID == "" {
			log.Println("EC2_INSTANCE_ID_OVERRIDE not set. EC2 metrics will likely fail unless on EC2.")
		} else {
			log.Printf("Using EC2_INSTANCE_ID_OVERRIDE: %s", instanceID)
		}
		// We return nil here, allowing the app to start even if metadata isn't found
		// (useful for local testing, but the /api/ec2-usage will fail).
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Metadata service returned non-200 status: %d. Using override or placeholder.", resp.StatusCode)
		instanceID = os.Getenv("EC2_INSTANCE_ID_OVERRIDE")
		if instanceID == "" {
			log.Println("EC2_INSTANCE_ID_OVERRIDE not set. EC2 metrics will likely fail.")
		} else {
			log.Printf("Using EC2_INSTANCE_ID_OVERRIDE: %s", instanceID)
		}
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read EC2 instance ID response: %w", err)
	}
	instanceID = string(body)

	log.Println("AWS CloudWatch client initialized. Instance ID:", instanceID)
	return nil
}

// --- GitHub Functions ---

// initGitHub initializes the GitHub client using a token from Vault.
func initGitHub() error {
	// We expect the path to be like 'kv/cloudpulse'
	githubToken, err := getSecret("kv/cloudpulse", "github_token")
	if err != nil {
		return fmt.Errorf("failed to get GitHub token from Vault: %w", err)
	}

	githubOwner = os.Getenv("GITHUB_OWNER")
	githubRepo = os.Getenv("GITHUB_REPO")
	if githubOwner == "" || githubRepo == "" {
		return fmt.Errorf("GITHUB_OWNER and GITHUB_REPO environment variables must be set")
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: githubToken})
	tc := oauth2.NewClient(context.Background(), ts)
	githubClient = github.NewClient(tc)

	log.Println("GitHub client initialized for repo:", githubOwner+"/"+githubRepo)
	return nil
}

// --- API Handlers ---

// ec2UsageHandler fetches basic CloudWatch metrics.
func ec2UsageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if cwClient == nil {
		http.Error(w, `{"error": "AWS client not initialized"}`, http.StatusInternalServerError)
		return
	}
	if instanceID == "" {
		http.Error(w, `{"error": "EC2 Instance ID not determined. Metrics unavailable."}`, http.StatusServiceUnavailable)
		log.Println("EC2 Instance ID is empty, cannot fetch metrics.")
		return
	}

	endTime := time.Now()
	startTime := endTime.Add(-10 * time.Minute)

	metricQueries := []types.MetricDataQuery{
		{
			Id: aws.String("cpu"), // <-- Use aws.String
			MetricStat: &types.MetricStat{
				Metric: &types.Metric{
					Namespace:  aws.String("AWS/EC2"),                                                              // <-- Use aws.String
					MetricName: aws.String("CPUUtilization"),                                                       // <-- Use aws.String
					Dimensions: []types.Dimension{{Name: aws.String("InstanceId"), Value: aws.String(instanceID)}}, // <-- Use aws.String
				},
				Period: aws.Int32(300),        // <-- Use aws.Int32
				Stat:   aws.String("Average"), // <-- Use aws.String
			},
			ReturnData: aws.Bool(true), // <-- Use aws.Bool
		},
		{
			Id: aws.String("netIn"), // <-- Use aws.String
			MetricStat: &types.MetricStat{
				Metric: &types.Metric{
					Namespace:  aws.String("AWS/EC2"),
					MetricName: aws.String("NetworkIn"),
					Dimensions: []types.Dimension{{Name: aws.String("InstanceId"), Value: aws.String(instanceID)}},
				},
				Period: aws.Int32(300), // <-- Use aws.Int32
				Stat:   aws.String("Sum"),
			},
			ReturnData: aws.Bool(true), // <-- Use aws.Bool
		},
		{
			Id: aws.String("netOut"), // <-- Use aws.String
			MetricStat: &types.MetricStat{
				Metric: &types.Metric{
					Namespace:  aws.String("AWS/EC2"),
					MetricName: aws.String("NetworkOut"),
					Dimensions: []types.Dimension{{Name: aws.String("InstanceId"), Value: aws.String(instanceID)}},
				},
				Period: aws.Int32(300), // <-- Use aws.Int32
				Stat:   aws.String("Sum"),
			},
			ReturnData: aws.Bool(true), // <-- Use aws.Bool
		},
	}

	resp, err := cwClient.GetMetricData(context.TODO(), &cloudwatch.GetMetricDataInput{
		StartTime:         &startTime,
		EndTime:           &endTime,
		MetricDataQueries: metricQueries,
		ScanBy:            types.ScanByTimestampDescending,
	})

	if err != nil {
		log.Printf("Error getting CloudWatch data: %v", err)
		http.Error(w, fmt.Sprintf(`{"error": "Error getting CloudWatch data: %v"}`, err), http.StatusInternalServerError)
		return
	}

	result := make(map[string]interface{})
	result["InstanceID"] = instanceID // Include instance ID

	for _, mdr := range resp.MetricDataResults {
		id := *mdr.Id
		if len(mdr.Values) > 0 {
			result[id] = mdr.Values[0]
			result[id+"_Timestamp"] = mdr.Timestamps[0].Format(time.RFC3339) // Use a standard format
		} else {
			result[id] = "N/A"
		}
	}
	if len(resp.MetricDataResults) == 0 {
		log.Println("CloudWatch GetMetricData returned no results.")
		result["message"] = "No metric data returned from CloudWatch."
	}

	json.NewEncoder(w).Encode(result)
}

// githubUsersHandler fetches collaborators from a GitHub repository.
func githubUsersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if githubClient == nil {
		http.Error(w, `{"error": "GitHub client not initialized"}`, http.StatusInternalServerError)
		return
	}

	users, _, err := githubClient.Repositories.ListCollaborators(
		context.Background(),
		githubOwner,
		githubRepo,
		&github.ListCollaboratorsOptions{ListOptions: github.ListOptions{PerPage: 100}},
	)

	if err != nil {
		log.Printf("Error getting GitHub users: %v", err)
		http.Error(w, fmt.Sprintf(`{"error": "Error getting GitHub users: %v"}`, err), http.StatusInternalServerError)
		return
	}

	type UserInfo struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
		HTMLURL   string `json:"html_url"`
		RoleName  string `json:"role_name"`
	}

	var userInfos []UserInfo
	for _, user := range users {
		userInfos = append(userInfos, UserInfo{
			Login:     safeDeref(user.Login),
			AvatarURL: safeDeref(user.AvatarURL),
			HTMLURL:   safeDeref(user.HTMLURL),
			RoleName:  safeDeref(user.RoleName),
		})
	}

	json.NewEncoder(w).Encode(userInfos)
}

// safeDeref safely dereferences a string pointer, returning "" if nil.
func safeDeref(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

// --- Main Application ---

func main() {
	log.Println("Starting CloudPulse Backend v3 (Corrected)...")

	if err := initVault(); err != nil {
		log.Fatalf("FATAL: Failed to initialize Vault: %v", err)
	}
	if err := initAWS(); err != nil {
		log.Fatalf("FATAL: Failed to initialize AWS SDK: %v", err)
	}
	if err := initGitHub(); err != nil {
		log.Fatalf("FATAL: Failed to initialize GitHub client: %v", err)
	}

	fs := http.FileServer(http.Dir("./frontend"))
	http.Handle("/", fs)

	http.HandleFunc("/api/ec2-usage", ec2UsageHandler)
	http.HandleFunc("/api/github-users", githubUsersHandler)
	http.HandleFunc("/api/free-tier-usage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Monitor AWS Free Tier usage via AWS Budgets and the Billing Console.",
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server listening on :%s...", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatalf("FATAL: Server failed to start: %v", err)
	}
}
