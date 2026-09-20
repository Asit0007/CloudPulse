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
	conf := vault.DefaultConfig() // Reads VAULT_ADDR from env (e.g., http://127.0.0.1:8201)

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
// corsOrigin is whatever CORS_ALLOW_ORIGIN is set to, read once at startup.
//
// These three endpoints used to send Access-Control-Allow-Origin: * unconditionally,
// which let any page on the internet read this deployment's AWS billing and
// CloudWatch figures from a visitor's browser. Nothing needs that: the Go binary
// serves frontend/ and the API from the same origin, so the dashboard is a
// same-origin caller and sends no Origin header worth answering. Set
// CORS_ALLOW_ORIGIN to a specific origin if the front end is ever hosted apart
// from the API. "*" still works, and still means what it says.
var corsOrigin = os.Getenv("CORS_ALLOW_ORIGIN")

// setCORS is a no-op unless CORS_ALLOW_ORIGIN is set. Vary matters even then:
// without it a shared cache can serve one origin's allowed response to another.
func setCORS(w http.ResponseWriter) {
	if corsOrigin == "" {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", corsOrigin)
	w.Header().Set("Vary", "Origin")
}

// apiError logs the real error and tells the caller only that the stage failed.
//
// These endpoints are unauthenticated and CORS is wide open, so whatever goes
// into http.Error is readable by anyone who can reach the box. AWS and GitHub
// SDK errors are not generic: an authorization failure reads "User:
// arn:aws:iam::<account>:user/<name> is not authorized to perform
// cloudwatch:GetMetricData", which hands an anonymous caller the account ID, the
// IAM principal and the exact permission that is missing. The operator needs
// that text; the internet does not, and it is already in the log.
func apiError(w http.ResponseWriter, stage string, err error) {
	log.Printf("%s: %v", stage, err)
	http.Error(w, fmt.Sprintf(`{"error": %q}`, stage+" failed"), http.StatusInternalServerError)
}

func ec2UsageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	setCORS(w)

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
		{//for CPU Utilization
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
		{//for Memory Utilization
			Id: aws.String("memUsed"),
			MetricStat: &types.MetricStat{
				Metric: &types.Metric{
					Namespace:  aws.String("CWAgent"),
					MetricName: aws.String("mem_used_percent"),
					Dimensions: []types.Dimension{
						{Name: aws.String("InstanceId"), Value: aws.String(instanceID)},
					},
				},
				Period: aws.Int32(300),
				Stat:   aws.String("Average"),
			},
			ReturnData: aws.Bool(true),
		},
		
		/*	{//for Disk Utilization
				Id: aws.String("diskUsed"),
				MetricStat: &types.MetricStat{
					Metric: &types.Metric{
						Namespace:  aws.String("CWAgent"),
						MetricName: aws.String("disk_used_percent"),
						Dimensions: []types.Dimension{
							//{Name: aws.String("InstanceId"), Value: aws.String(instanceID)},
							//{Name: aws.String("path"), Value: aws.String("/")}, // root disk
							//{Name: aws.String("device"), Value: aws.String("nvme0n1p1")},
							//{Name: aws.String("path"), Value: aws.String("/")}, // root disk
							//{Name: aws.String("fstype"), Value: aws.String("xfs")}, // or "ext4" depending on your AMI
						},
					},
					Period: aws.Int32(300),
					Stat:   aws.String("Average"),
				},
				ReturnData: aws.Bool(true),
			},*/

		{//for Network In
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
		{//for Network Out
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
		apiError(w, "Error getting CloudWatch data", err)
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

// freeTierUsageHandler fetches EC2 hours and Data Transfer Out for the current month.
func freeTierUsageHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    setCORS(w)

    if cwClient == nil {
        http.Error(w, `{"error": "AWS client not initialized"}`, http.StatusInternalServerError)
        return
    }

    // Calculate start and end of current month (UTC)
    now := time.Now().UTC()
    startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
    endOfMonth := now

    // EC2 Hours (sum of "CPUUtilization" datapoints, but AWS Free Tier is based on instance-hours, not CPU)
    // We'll use "RunningHours" metric if available, otherwise count running hours by instance state.
    // For demo, we'll use "CPUUtilization" sample count as a proxy (not exact).

    // Data Transfer Out (sum of "NetworkOut" for all EC2 instances)
    // For simplicity, this demo fetches for the current instance only.
    // For full account, you'd need to list all instances and sum.

    // Fetch NetworkOut for this instance
    netOutInput := &cloudwatch.GetMetricStatisticsInput{
        Namespace:  aws.String("AWS/EC2"),
        MetricName: aws.String("NetworkOut"),
        Dimensions: []types.Dimension{
            {Name: aws.String("InstanceId"), Value: aws.String(instanceID)},
        },
        StartTime: &startOfMonth,
        EndTime:   &endOfMonth,
        Period:    aws.Int32(86400), // 1 day
        Statistics: []types.Statistic{
            types.StatisticSum,
        },
    }
    netOutResp, err := cwClient.GetMetricStatistics(context.TODO(), netOutInput)
    if err != nil {
        apiError(w, "Failed to get NetworkOut", err)
        return
    }
    var totalNetOut float64
    for _, dp := range netOutResp.Datapoints {
        totalNetOut += *dp.Sum
    }
    // Convert bytes to GB
    totalNetOutGB := totalNetOut / (1024 * 1024 * 1024)
    dataTransferOutRemaining := 100.0 - totalNetOutGB
    if dataTransferOutRemaining < 0 {
        dataTransferOutRemaining = 0
    }

    // EC2 Hours Used (approximate: count days * 24 if instance running all month)
    // For demo, we'll just show hours since start of month for this instance
    hoursUsed := endOfMonth.Sub(startOfMonth).Hours()
    hoursRemaining := 750.0 - hoursUsed
    if hoursRemaining < 0 {
        hoursRemaining = 0
    }

    result := map[string]interface{}{
        "ec2HoursUsed":             int(hoursUsed),
        "ec2HoursRemaining":        int(hoursRemaining),
        "dataTransferOutUsed":      fmt.Sprintf("%.2f", totalNetOutGB),
        "dataTransferOutRemaining": fmt.Sprintf("%.2f", dataTransferOutRemaining),
        "timestamp":                now.Format(time.RFC3339),
    }

    json.NewEncoder(w).Encode(result)
}

// githubUsersHandler fetches collaborators from a GitHub repository.
func githubUsersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	setCORS(w)

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
		apiError(w, "Error getting GitHub users", err)
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
	http.HandleFunc("/api/free-tier-usage", freeTierUsageHandler)
	/*http.HandleFunc("/api/free-tier-usage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Monitor AWS Free Tier usage via AWS Budgets and the Billing Console.",
		})
	})*/

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// http.ListenAndServe applies no timeouts at all: a client that opens a
	// connection and never finishes its request headers holds a goroutine and an
	// fd until the process dies, which is Slowloris with no tooling required.
	// These four are the ones net/http leaves at zero.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           nil, // DefaultServeMux, as registered above
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second, // Cost Explorer is the slow one
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("Server listening on :%s...", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("FATAL: Server failed to start: %v", err)
	}
}

// finally the application is ready to run