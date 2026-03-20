package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"cli-scrobbler/internal/discogs"
	"cli-scrobbler/internal/lastfm"
	"cli-scrobbler/internal/model"
)

func promptIndex(reader *bufio.Reader, out io.Writer, max int) (int, error) {
	for {
		fmt.Fprintf(out, "Enter selection [1-%d]: ", max)
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("read selection: %w", err)
		}

		value, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || value < 1 || value > max {
			fmt.Fprintln(out, "Please enter a valid selection.")
			continue
		}

		return value - 1, nil
	}
}

func promptDuration(reader *bufio.Reader, out io.Writer, track model.Track) (time.Duration, error) {
	for {
		fmt.Fprintf(out, "Enter duration for %s %q (mm:ss, hh:mm:ss, or seconds): ", track.Position, track.Title)
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, fmt.Errorf("read duration: %w", err)
		}

		duration, err := parsePromptDuration(strings.TrimSpace(line))
		if err != nil {
			fmt.Fprintf(out, "Invalid duration: %v\n", err)
			continue
		}
		return duration, nil
	}
}

func parsePromptDuration(value string) (time.Duration, error) {
	if value == "" {
		return 0, errors.New("duration is required")
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0, errors.New("duration must be positive")
		}
		return time.Duration(seconds) * time.Second, nil
	}

	return discogs.ParseDuration(value)
}

func parseStartedAt(value string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}

	for _, format := range formats {
		var (
			parsed time.Time
			err    error
		)

		if format == time.RFC3339 {
			parsed, err = time.Parse(format, value)
		} else {
			parsed, err = time.ParseInLocation(format, value, time.Local)
		}
		if err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported --started-at value %q", value)
}

func promptStartedAt(reader *bufio.Reader, out io.Writer) (time.Time, error) {
	for {
		now := time.Now().Format("2006-01-02 15:04")
		value, err := promptOptionalValue(reader, out, fmt.Sprintf("When did you start playing it? (blank uses now: %s)", now), "")
		if err != nil {
			return time.Time{}, err
		}

		if value == "" {
			return time.Now(), nil
		}

		startedAt, err := parseStartedAt(value)
		if err != nil {
			fmt.Fprintf(out, "Invalid start time: %v\n", err)
			continue
		}
		return startedAt, nil
	}
}

func promptRequiredValue(reader *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	for {
		value, err := promptValue(reader, out, label, defaultValue)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
		fmt.Fprintln(out, "A value is required.")
	}
}

func promptSecretValue(reader *bufio.Reader, out io.Writer, label, currentValue string) (string, error) {
	for {
		if strings.TrimSpace(currentValue) != "" {
			fmt.Fprintf(out, "%s [stored]: ", label)
		} else {
			fmt.Fprintf(out, "%s: ", label)
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("read input: %w", err)
		}

		value := strings.TrimSpace(line)
		if value == "" {
			if strings.TrimSpace(currentValue) != "" {
				return strings.TrimSpace(currentValue), nil
			}
			fmt.Fprintln(out, "A value is required.")
			continue
		}

		return value, nil
	}
}

func promptLastFMSessionKey(reader *bufio.Reader, out io.Writer, apiKey, apiSecret, currentValue string) (string, error) {
	useGuidedFlow := true
	if strings.TrimSpace(currentValue) != "" {
		keepCurrent, err := promptYesNo(reader, out, "Keep the currently stored Last.fm session key?", true)
		if err != nil {
			return "", err
		}
		if keepCurrent {
			return strings.TrimSpace(currentValue), nil
		}

		useGuidedFlow, err = promptYesNo(reader, out, "Generate a new Last.fm session key through the browser-based auth flow?", true)
		if err != nil {
			return "", err
		}
	} else {
		var err error
		useGuidedFlow, err = promptYesNo(reader, out, "Generate a Last.fm session key now through the browser-based auth flow?", true)
		if err != nil {
			return "", err
		}
	}

	if useGuidedFlow {
		return guideLastFMSessionKey(reader, out, apiKey, apiSecret)
	}

	return promptSecretValue(reader, out, "Last.fm session key", currentValue)
}

func guideLastFMSessionKey(reader *bufio.Reader, out io.Writer, apiKey, apiSecret string) (string, error) {
	client := lastfm.NewClient(apiKey, apiSecret, "")
	token, err := client.GetAuthToken(context.Background())
	if err != nil {
		return "", err
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Open this URL in your browser and approve access for the application:")
	fmt.Fprintln(out, client.AuthURL(token))
	fmt.Fprintln(out)

	if _, err := promptRequiredValue(reader, out, "Press Enter after approving access in Last.fm", "ready"); err != nil {
		return "", err
	}

	sessionKey, err := client.GetSessionKey(context.Background(), token)
	if err != nil {
		return "", err
	}

	fmt.Fprintln(out, "Last.fm session key obtained successfully.")
	return sessionKey, nil
}

func promptOptionalValue(reader *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	return promptValue(reader, out, label, defaultValue)
}

func promptValue(reader *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}

	value := strings.TrimSpace(line)
	if value == "" {
		return strings.TrimSpace(defaultValue), nil
	}

	return value, nil
}

func promptYesNo(reader *bufio.Reader, out io.Writer, label string, defaultYes bool) (bool, error) {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}

	for {
		fmt.Fprintf(out, "%s %s: ", label, suffix)
		line, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("read input: %w", err)
		}

		value := strings.TrimSpace(strings.ToLower(line))
		if value == "" {
			return defaultYes, nil
		}
		if value == "y" || value == "yes" {
			return true, nil
		}
		if value == "n" || value == "no" {
			return false, nil
		}

		fmt.Fprintln(out, "Please answer yes or no.")
	}
}
