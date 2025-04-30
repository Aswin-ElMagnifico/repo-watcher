package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the tool configuration.
type Config struct {
	RepoURL      string `yaml:"repo_url"`
	RepoName     string `yaml:"repo_name"`
	AuthToken    string `yaml:"auth_token,omitempty"`
	PollInterval int    `yaml:"poll_interval_seconds"`
	Branch       string `yaml:"branch,omitempty"`
	Notifier     struct {
		Type       string `yaml:"type"`
		WebhookURL string `yaml:"webhook_url"`
	} `yaml:"notifier"`
}

// Notifier is the interface for sending messages.
type Notifier interface {
	Notify(msg string) error
}

// SlackNotifier sends messages to Slack via webhook.
type SlackNotifier struct {
	WebhookURL string
}

func (s SlackNotifier) Notify(msg string) error {
	return nil
	payload := fmt.Sprintf(`{"text":"%s"}`, msg)
	cmd := exec.Command("bash", "-c",
		fmt.Sprintf(`curl -fsSL -X POST -H 'Content-type: application/json' --data '%s' %s`,
			payload, s.WebhookURL))
	return cmd.Run()
}

// loadConfig reads and parses config.yaml.
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	return &cfg, nil
}

// runCommands runs a list of shell commands, returning on first error.
func runCommands(cmds []string, repoDir string) error {
	currentPath, _ := os.Getwd()
	os.Chdir(repoDir)
	defer os.Chdir(currentPath)
	for _, c := range cmds {
		cmd := exec.Command("bash", "-c", c)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("'%s' failed: %w", c, err)
		}
	}
	return nil
}

// readSteps reads .internal/steps.yaml from the repoDir.
func readSteps(repoDir string) (start, stop []string, err error) {
	data, err := os.ReadFile(filepath.Join(repoDir, ".internal", "steps.yaml"))
	if err != nil {
		return
	}
	var y struct {
		Start []string `yaml:"start"`
		Stop  []string `yaml:"stop"`
	}
	if err = yaml.Unmarshal(data, &y); err != nil {
		return
	}
	return y.Start, y.Stop, nil
}

// git runs git commands in repoDir.
func git(repoDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	fmt.Println(string(out))
	return strings.TrimSpace(string(out)), err
}

// cloneRepo clones into targetDir (must not exist).
func cloneRepo(cfg *Config, targetDir string) error {
	url := cfg.RepoURL
	if cfg.AuthToken != "" {
		// insert token: https://token@github.com/owner/repo.git
		url = strings.Replace(url, "https://", fmt.Sprintf("https://%s@", cfg.AuthToken), 1)
	}
	_, err := git("", "clone", "-b", cfg.Branch, url, targetDir)
	return err
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: repo-watcher <config.yaml>")
		os.Exit(1)
	}
	cfgPath := os.Args[1]
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// Prepare notifier
	var notifier Notifier
	switch strings.ToLower(cfg.Notifier.Type) {
	case "slack":
		notifier = SlackNotifier{WebhookURL: cfg.Notifier.WebhookURL}
	// case "telegram": ...
	// case "discord": ...
	default:
		fmt.Fprintf(os.Stderr, "unsupported notifier type: %s\n", cfg.Notifier.Type)
		os.Exit(1)

	}

	// Clone repo
	repoDir := cfg.RepoName
	if cfg.RepoName == "" {
		repoDir = "./generic-name-repo"
	}

	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		if err := cloneRepo(cfg, repoDir); err != nil {
			notifier.Notify(fmt.Sprintf("[RepoWatcher] Failed to clone repo: %v", err))
			fmt.Fprintf(os.Stderr, "clone error: %v\n", err)
			os.Exit(1)
		}
		notifier.Notify("[RepoWatcher] Repository cloned.")
	}

	// Read steps.yaml
	startCmds, stopCmds, err := readSteps(repoDir)
	if err != nil {
		notifier.Notify(fmt.Sprintf("[RepoWatcher] Failed to read steps.yaml: %v", err))
		fmt.Fprintf(os.Stderr, "steps.yaml error: %v\n", err)
		os.Exit(1)
	}

	// Initial start
	if err := runCommands(startCmds, repoDir); err != nil {
		notifier.Notify(fmt.Sprintf("[RepoWatcher] Failed to execute start: %v", err))
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		notifier.Notify("[RepoWatcher] Shutting down, running stop commands...")
		runCommands(stopCmds, repoDir)
		cancel()
	}()

	// Polling loop
	ticker := time.NewTicker(time.Duration(cfg.PollInterval) * time.Second)
	defer ticker.Stop()

	// Track current HEAD
	prevHead, _ := git(repoDir, "rev-parse", "HEAD")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// fetch and check
			_, err = git(repoDir, "fetch")
			if err != nil {
				notifier.Notify(fmt.Sprintf("[RepoWatcher] git fetch failed: %v", err))
				continue
			}
			currHead, err := git(repoDir, "rev-parse", fmt.Sprintf("origin/%s", cfg.Branch))
			if err != nil {
				notifier.Notify(fmt.Sprintf("[RepoWatcher] rev-parse failed: %v", err))
				continue
			}

			if currHead != prevHead {

				_, err := git(repoDir, "pull")
				if err != nil {
					notifier.Notify(fmt.Sprintf("[RepoWatcher] git pull failed: %v", err))
					continue
				}

				notifier.Notify(fmt.Sprintf("[RepoWatcher] New commit %s detected. Restarting...", currHead))

				startCmds, stopCmds, err := readSteps(repoDir)
				if err != nil {
					notifier.Notify(fmt.Sprintf("[RepoWatcher] Failed to read steps.yaml: %v", err))
					fmt.Fprintf(os.Stderr, "steps.yaml error: %v\n", err)
					os.Exit(1)
				}

				// stop
				if err := runCommands(stopCmds, repoDir); err != nil {
					notifier.Notify(fmt.Sprintf("[RepoWatcher] stop failed: %v", err))
				}
				// pull

				// start
				if err := runCommands(startCmds, repoDir); err != nil {
					notifier.Notify(fmt.Sprintf("[RepoWatcher] start failed: %v", err))
				} else {
					notifier.Notify("[RepoWatcher] Successfully restarted service.")
				}
				prevHead = currHead
			}
		}
	}
}
