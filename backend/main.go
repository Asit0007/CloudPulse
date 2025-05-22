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
	"github.com/hashicorp/vault/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/oauth2"
)

// Config structure to hold all environment configuration
type Config struct {
	Stage              string
	AWSRegion          string
	LogLevel           string
	Port               string
	VaultAddr          string
	VaultToken         string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
}

// Prometheus metrics
var (
	requestCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudpulse_requests_total",
			Help: "Total number of requests handled",
		},
		[]string{"endpoint", "stage"},
	)

	requestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cloudpulse_request_duration_seconds",
			Help:    "Request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint", "stage"},
	)
)

// Register Prometheus metrics
func init() {
	prometheus.MustRegister(requestCounter)
	prometheus.MustRegister(requestLatency)
}

// Load environment configuration with validation
func loadConfig() Config {
	cfg := Config{
		Stage:      getEnv("STAGE", "testing"),
		AWSRegion:  getEnv("AWS_REGION", "us-east-1"),
		LogLevel:   getEnv("LOG_LEVEL", "info"),
		Port:       getEnv("PORT", "8080"),
		VaultAddr:  getEnv("VAULT_ADDR", "http://localhost:8200"),
		VaultToken: getEnv("VAULT_TOKEN", "root"),
	}

	// Validate stage value
	allowedStages := []string{"testing", "staging", "production"}
	isValidStage := false
	for _, stage := range allowedStages {
		if cfg.Stage == stage {
			isValidStage = true
			break
		}
	}
	if !isValidStage {
		log.Fatalf("Invalid STAGE: %s. Must be one of: %v", cfg.Stage, allowedStages)
	}

	// Ensure Vault credentials are provided
	if cfg.VaultAddr == "" || cfg.VaultToken == "" {
		log.Fatal("VAULT_ADDR and VAULT_TOKEN are required")
	}

	return cfg
}

// Retrieve environment variable with default fallback
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// Fetch secret from Vault for the given path
func getSecretFromVault(vaultAddr, vaultToken, path string) (string, error) {
	config := &api.Config{Address: vaultAddr}
	client, err := api.NewClient(config)
	if err != nil {
		return "", fmt.Errorf("failed to create Vault client: %v", err)
	}

	client.SetToken(vaultToken)
	secret, err := client.Logical().Read(path)
	if err != nil {
		return "", fmt.Errorf("failed to read secret from Vault at %s: %v", path, err)
	}

	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("no secret found at path %s", path)
	}

	value, ok := secret.Data["value"].(string)
	if !ok {
		return "", fmt.Errorf("secret at path %s does not contain a 'value' field", path)
	}

	return value, nil
}

func main() {
	cfg := loadConfig()
	log.Printf("Starting CloudPulse in %s stage", cfg.Stage)

	// Retrieve secrets from Vault
	secretPathPrefix := fmt.Sprintf("secret/cloudpulse/%s", cfg.Stage)

	githubToken, err := getSecretFromVault(cfg.VaultAddr, cfg.VaultToken, secretPathPrefix+"/GITHUB_TOKEN")
	if err != nil {
		log.Fatalf("Failed to fetch GITHUB_TOKEN: %v", err)
	}

	cfg.AWSAccessKeyID, err = getSecretFromVault(cfg.VaultAddr, cfg.VaultToken, secretPathPrefix+"/AWS_ACCESS_KEY_ID")
	if err != nil {
		log.Fatalf("Failed to fetch AWS_ACCESS_KEY_ID: %v", err)
	}

	cfg.AWSSecretAccessKey, err = getSecretFromVault(cfg.VaultAddr, cfg.VaultToken, secretPathPrefix+"/AWS_SECRET_ACCESS_KEY")
	if err != nil {
		log.Fatalf("Failed to fetch AWS_SECRET_ACCESS_KEY: %v", err)
	}

	// Initialize GitHub client with token authentication
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: githubToken})
	tc := oauth2.NewClient(ctx, ts)
	githubClient := github.NewClient(tc)

	// Load AWS SDK config using credentials from Vault
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.AWSRegion),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     cfg.AWSAccessKeyID,
				SecretAccessKey: cfg.AWSSecretAccessKey,
			}, nil
		})),
	)
	if err != nil {
		log.Fatalf("Unable to load AWS SDK config: %v", err)
	}

	awsClient := costexplorer.NewFromConfig(awsCfg)

	// Create default frontend if not exists
	if _, err := os.Stat("frontend"); os.IsNotExist(err) {
		log.Println("Creating default frontend...")
		os.MkdirAll("frontend", 0755)
		os.WriteFile("frontend/index.html", []byte("<h1>Welcome to CloudPulse</h1><p>Go to /api/costs or /api/github.</p>"), 0644)
	}

	// Expose Prometheus metrics endpoint
	http.Handle("/metrics", promhttp.Handler())

	// Handler for AWS cost explorer API
	http.HandleFunc("/api/costs", func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		requestCounter.WithLabelValues("/api/costs", cfg.Stage).Inc()
		defer func() {
			duration := time.Since(startTime).Seconds()
			requestLatency.WithLabelValues("/api/costs", cfg.Stage).Observe(duration)
		}()

		// Define 30-day time range for cost retrieval
		end := time.Now()
		start := end.AddDate(0, 0, -30)

		input := &costexplorer.GetCostAndUsageInput{
			TimePeriod: &types.DateInterval{
				Start: aws.String(start.Format("2006-01-02")),
				End:   aws.String(end.Format("2006-01-02")),
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
			http.Error(w, fmt.Sprintf("Error fetching AWS costs: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	// Handler for GitHub user data
	http.HandleFunc("/api/github", func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		requestCounter.WithLabelValues("/api/github", cfg.Stage).Inc()
		defer func() {
			duration := time.Since(startTime).Seconds()
			requestLatency.WithLabelValues("/api/github", cfg.Stage).Observe(duration)
		}()

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
		json.NewEncoder(w).Encode(response)
	})

	// Serve static frontend files
	http.Handle("/", http.FileServer(http.Dir("frontend")))

	// Start HTTP server
	log.Printf("Server starting on :%s", cfg.Port)
	http.ListenAndServe(":"+cfg.Port, nil)
}
