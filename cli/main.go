package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Default server. Override with `ICEBERG_SERVER` or by saving one in config.
const defaultServer = "https://iceberg.sh"

var topicRe = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

type config struct {
	Topic  string `json:"topic"`
	Server string `json:"server"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "iceberg: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}

	switch args[0] {
	case "topic":
		return cmdTopic(args[1:])
	case "push":
		return cmdPush(args[1:])
	case "pull":
		return cmdPull(args[1:])
	case "open":
		return cmdOpen(args[1:])
	case "delete":
		return cmdDelete(args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q (try: topic, push, pull, open, delete)", args[0])
	}
}

// iceberg delete [name]  — clear a topic by sending an empty update (asks first).
func cmdDelete(args []string) error {
	topicFlag, rest, err := extractTopicFlag(args)
	if err != nil {
		return err
	}
	explicit := topicFlag
	if explicit == "" && len(rest) > 0 {
		explicit = rest[0]
	}
	topic, err := resolveTopic(explicit)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "clear topic %q? [y/N] ", topic)
	var resp string
	fmt.Scanln(&resp)
	if strings.ToLower(strings.TrimSpace(resp)) != "y" {
		fmt.Fprintln(os.Stderr, "aborted")
		return nil
	}

	url := serverURL() + "/" + topic
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Post(url, "application/octet-stream", strings.NewReader(""))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(res.Body)
		return fmt.Errorf("server said %s: %s", res.Status, strings.TrimSpace(string(msg)))
	}

	fmt.Fprintf(os.Stderr, "cleared %s\n", topic)
	return nil
}

// iceberg open [name]  — open the topic's viewer page in a browser.
func cmdOpen(args []string) error {
	topicFlag, rest, err := extractTopicFlag(args)
	if err != nil {
		return err
	}
	explicit := topicFlag
	if explicit == "" && len(rest) > 0 {
		explicit = rest[0]
	}
	topic, err := resolveTopic(explicit)
	if err != nil {
		return err
	}

	url := serverURL() + "/v/" + topic
	fmt.Fprintln(os.Stderr, "opening "+url)
	return openBrowser(url)
}

// openBrowser launches the OS default browser at url.
func openBrowser(url string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		name, args = "xdg-open", []string{url}
	}
	return exec.Command(name, args...).Start()
}

// iceberg topic [name]  — set the default topic, or print it.
func cmdTopic(args []string) error {
	cfg := loadConfig()

	if len(args) == 0 {
		if cfg.Topic == "" {
			fmt.Println("no default topic set — run: iceberg topic <name>")
		} else {
			fmt.Println(cfg.Topic)
		}
		return nil
	}

	name := args[0]
	if !topicRe.MatchString(name) {
		return fmt.Errorf("bad topic %q (lowercase letters, digits, hyphens; 1-64 chars)", name)
	}
	cfg.Topic = name
	if err := saveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("default topic set to %q\n", name)
	return nil
}

// iceberg push [name]  — POST stdin to the topic.
func cmdPush(args []string) error {
	topicFlag, rest, err := extractTopicFlag(args)
	if err != nil {
		return err
	}
	topic, err := resolveTopic(topicFlag)
	if err != nil {
		return err
	}

	// positional arg = body, else stdin (multi-word must be quoted)
	var body []byte
	switch len(rest) {
	case 0:
		body, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
	case 1:
		body = []byte(rest[0])
	default:
		return fmt.Errorf("too many arguments — quote multi-word text: iceberg push \"%s\"", strings.Join(rest, " "))
	}

	url := serverURL() + "/" + topic
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(url, "application/octet-stream", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server said %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	fmt.Fprintf(os.Stderr, "pushed %d bytes → %s\n", len(body), topic)
	return nil
}

// iceberg pull [name]  — GET the topic, write to stdout.
func cmdPull(args []string) error {
	topicFlag, rest, err := extractTopicFlag(args)
	if err != nil {
		return err
	}
	// positional = topic
	explicit := topicFlag
	if explicit == "" && len(rest) > 0 {
		explicit = rest[0]
	}
	topic, err := resolveTopic(explicit)
	if err != nil {
		return err
	}

	url := serverURL() + "/" + topic
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("topic %q is empty", topic)
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server said %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	os.Stdout.Write(body)
	// newline only for a terminal — keeps pipes byte-exact
	if isTerminal(os.Stdout) {
		fmt.Println()
	}
	return nil
}

// resolveTopic uses an explicit topic (from -t or a positional) if given,
// otherwise falls back to the saved default.
func resolveTopic(explicit string) (string, error) {
	if explicit != "" {
		if !topicRe.MatchString(explicit) {
			return "", fmt.Errorf("bad topic %q (lowercase letters, digits, hyphens; 1-64 chars)", explicit)
		}
		return explicit, nil
	}
	cfg := loadConfig()
	if cfg.Topic == "" {
		return "", fmt.Errorf("no topic: pass --topic <name> or run 'iceberg topic <name>'")
	}
	return cfg.Topic, nil
}

// extractTopicFlag splits -t/--topic out of args, returning it and the rest.
func extractTopicFlag(args []string) (topic string, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-t" || a == "--topic":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s needs a value", a)
			}
			topic = args[i+1]
			i++
		case strings.HasPrefix(a, "-t="):
			topic = strings.TrimPrefix(a, "-t=")
		case strings.HasPrefix(a, "--topic="):
			topic = strings.TrimPrefix(a, "--topic=")
		default:
			rest = append(rest, a)
		}
	}
	return topic, rest, nil
}

// serverURL resolves the server: config → env → default.
func serverURL() string {
	if cfg := loadConfig(); cfg.Server != "" {
		return strings.TrimRight(cfg.Server, "/")
	}
	if s := os.Getenv("ICEBERG_SERVER"); s != "" {
		return strings.TrimRight(s, "/")
	}
	return defaultServer
}

// --- config file helpers ---

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "iceberg", "config.json"), nil
}

func loadConfig() config {
	var cfg config
	path, err := configPath()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg // missing/unreadable → empty config
	}
	json.Unmarshal(data, &cfg)
	return cfg
}

func saveConfig(cfg config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// isTerminal reports whether f is a character device (a terminal) vs a pipe/file.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func usage() {
	fmt.Print(`iceberg — copy-paste bridge between machines

usage:
  iceberg topic [name]       set or show the default topic
  iceberg push [text]        send text (or stdin) to a topic
  iceberg pull [name]        print a topic to stdout
  iceberg open [name]        open the topic's viewer page in a browser
  iceberg delete [name]      clear a topic (empty update, asks to confirm)

  -t, --topic <name>         target topic (overrides the default)

examples:
  iceberg topic notes
  iceberg push "hello"           # inline text → default topic
  iceberg push hi -t other       # inline text → topic "other"
  echo "hi" | iceberg push       # stdin → default topic
  iceberg pull                   # default topic
  iceberg pull other             # explicit topic
  iceberg open                   # open default topic in browser

server: ` + serverURL() + `  (override with ICEBERG_SERVER)
`)
}
