package main

import (
	"context"
	"encoding/json"
	"fmt" // Required for io.ReadAll
	"log"
	"net/http"
	"os"
	"strings"
	"time"

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
// VAULT_ADDR and VAULT_TOKEN must be set as environment variables.
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
// Assumes secrets are stored at a path like 'kv/data/cloudpulse' (Vault CLI uses 'kv/cloudpulse').
// The actual path for KVv2 API is 'kv/data/your-path'.
func getSecret(secretPath, key string) (string, error) {
	if vaultClient == nil {
		return "", fmt.Errorf("vault client not initialized")
	}

	// For KVv2, the API path includes "/data/" after the mount path.
	// If your mount path is 'kv', and you put secrets at 'cloudpulse',
	// the API path is 'kv/data/cloudpulse'.
	log.Printf("Fetching secret '%s' from Vault path '%s'\n", key, secretPath)
	secret, err := vaultClient.KVv2(strings.Split(secretPath, "/")[0]).Get(context.Background(), strings.Join(strings.Split(secretPath, "/")[1:], "/"))
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

// initAWS initializes the AWS CloudWatch client.
// It relies on the IAM role attached to the EC2 instance.
func initAWS() error {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	cwClient = cloudwatch.NewFromConfig(cfg)

	// Fetch instance ID from EC2 metadata service (free)
	// This is a common way to get the instance ID from within the EC2 instance.
	// Ensure the EC2 instance has network access to the metadata service (169.254.169.254).
	metadataClient := config.NewEC2MetadataClient(cfg)
	id, err := metadataClient.GetMetadata(context.TODO(), &config.EC2GetMetadataInput{
		Path: "instance-id",
	})
	if err != nil {
		// Fallback for local testing or if metadata service is unavailable
		log.Printf("Could not fetch instance ID from metadata service: %v. Using 'i-placeholder' for local testing.", err)
		instanceID = os.Getenv("EC2_INSTANCE_ID_OVERRIDE") // Allow override for local
		if instanceID == "" {
			log.Println("EC2_INSTANCE_ID_OVERRIDE not set, EC2 metrics will likely fail if not on EC2.")
			// It's okay to proceed, the /api/ec2-usage endpoint will just return an error or no data.
		} else {
			log.Printf("Using EC2_INSTANCE_ID_OVERRIDE: %s", instanceID)
		}
	} else {
		instanceID = id
	}

	log.Println("AWS CloudWatch client initialized. Instance ID determined as:", instanceID)
	return nil
}

// --- GitHub Functions ---

// initGitHub initializes the GitHub client using a token from Vault.
func initGitHub() error {
	// Path in Vault: kv/cloudpulse, key: github_token
	// The getSecret function expects the full path for KVv2, e.g., "kv/data/cloudpulse"
	// Let's assume the mount is 'kv' and the secret path within that mount is 'cloudpulse'
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

// ec2UsageHandler fetches basic CloudWatch metrics for the EC2 instance.
// Uses GetMetricData for efficiency (one API call for multiple metrics).
// Fetches 5-minute average CPUUtilization and sum of NetworkIn/Out.
func ec2UsageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Allow all for simplicity

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
	startTime := endTime.Add(-10 * time.Minute) // Look at the last 10 minutes for better chance of data

	metricQueries := []types.MetricDataQuery{
		{
			Id: github.String("cpu"),
			MetricStat: &types.MetricStat{
				Metric: &types.Metric{
					Namespace:  github.String("AWS/EC2"),
					MetricName: github.String("CPUUtilization"),
					Dimensions: []types.Dimension{{Name: github.String("InstanceId"), Value: github.String(instanceID)}},
				},
				Period: github.Int32(300), // 5-minute period
				Stat:   github.String("Average"),
			},
			ReturnData: github.Bool(true),
		},
		{
			Id: github.String("netIn"),
			MetricStat: &types.MetricStat{
				Metric: &types.Metric{
					Namespace:  github.String("AWS/EC2"),
					MetricName: github.String("NetworkIn"),
					Dimensions: []types.Dimension{{Name: github.String("InstanceId"), Value: github.String(instanceID)}},
				},
				Period: github.Int32(300),
				Stat:   github.String("Sum"),
			},
			ReturnData: github.Bool(true),
		},
		{
			Id: github.String("netOut"),
			MetricStat: &types.MetricStat{
				Metric: &types.Metric{
					Namespace:  github.String("AWS/EC2"),
					MetricName: github.String("NetworkOut"),
					Dimensions: []types.Dimension{{Name: github.String("InstanceId"), Value: github.String(instanceID)}},
				},
				Period: github.Int32(300),
				Stat:   github.String("Sum"),
			},
			ReturnData: github.Bool(true),
		},
	}

	resp, err := cwClient.GetMetricData(context.TODO(), &cloudwatch.GetMetricDataInput{
		StartTime:         &startTime,
		EndTime:           &endTime,
		MetricDataQueries: metricQueries,
		ScanBy:            types.ScanByTimestampDescending, // Get latest data first
	})

	if err != nil {
		log.Printf("Error getting CloudWatch data: %v", err)
		http.Error(w, fmt.Sprintf(`{"error": "Error getting CloudWatch data: %v"}`, err), http.StatusInternalServerError)
		return
	}

	result := make(map[string]interface{})
	result["InstanceID"] = instanceID // Include instance ID in response

	for _, mdr := range resp.MetricDataResults {
		id := *mdr.Id
		if len(mdr.Values) > 0 && len(mdr.Timestamps) > 0 {
			result[id] = mdr.Values[0] // Get the first (latest) value
			result[id+"_Timestamp"] = mdr.Timestamps[0].Format(time.RFC3339)
		} else {
			result[id] = "N/A"
			log.Printf("No data points returned for metric: %s", id)
		}
	}
	if len(resp.MetricDataResults) == 0 {
		log.Println("CloudWatch GetMetricData returned no results.")
		result["message"] = "No metric data returned from CloudWatch. This can happen if the instance is new or metrics are not yet available."
	}

	json.NewEncoder(w).Encode(result)
}

// githubUsersHandler fetches collaborators from the configured GitHub repository.
// Requires a GitHub token with 'repo' scope (or 'read:org' for org repos).
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
		&github.ListCollaboratorsOptions{ListOptions: github.ListOptions{PerPage: 100}}, // Get up to 100
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
		userInfo := UserInfo{
			Login:     safeDeref(user.Login),
			AvatarURL: safeDeref(user.AvatarURL),
			HTMLURL:   safeDeref(user.HTMLURL),
			RoleName:  safeDeref(user.RoleName),
		}
		userInfos = append(userInfos, userInfo)
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
	log.Println("Starting CloudPulse Backend v2...")

	// 1. Initialize Vault (Needs VAULT_ADDR & VAULT_TOKEN env vars)
	// VAULT_ADDR typically http://127.0.0.1:8200 if Vault container is port-mapped on host
	if err := initVault(); err != nil {
		log.Fatalf("FATAL: Failed to initialize Vault: %v. Ensure VAULT_ADDR and VAULT_TOKEN are set.", err)
	}

	// 2. Initialize AWS (Needs IAM Role on EC2 or local credentials)
	if err := initAWS(); err != nil {
		log.Fatalf("FATAL: Failed to initialize AWS SDK: %v", err)
	}

	// 3. Initialize GitHub (Needs GITHUB_OWNER & GITHUB_REPO env vars, and token in Vault)
	// The Vault path for github_token is assumed to be 'kv/cloudpulse'
	if err := initGitHub(); err != nil {
		log.Fatalf("FATAL: Failed to initialize GitHub client: %v", err)
	}

	// 4. Setup HTTP Server and Routes
	// Serve static files from the "./frontend" directory
	fs := http.FileServer(http.Dir("./frontend"))
	http.Handle("/", fs) // Serve index.html and other assets at the root

	// API Endpoints
	http.HandleFunc("/api/ec2-usage", ec2UsageHandler)
	http.HandleFunc("/api/github-users", githubUsersHandler)
	// Placeholder for Free Tier info - frontend handles this with links
	http.HandleFunc("/api/free-tier-usage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Monitor AWS Free Tier usage via AWS Budgets and the Billing Console. Programmatic access can incur costs.",
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port
	}

	log.Printf("Server listening on :%s...", port)
	// The server will serve static files from './frontend' and API endpoints under '/api/'
	err := http.ListenAndServe(":"+port, nil) // Use DefaultServeMux
	if err != nil {
		log.Fatalf("FATAL: Server failed to start: %v", err)
	}
}
