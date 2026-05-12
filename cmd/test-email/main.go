package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	mailerpkg "cloud-backend/pkg/mailer"
)

func main() {
	port, _ := strconv.Atoi(envOr("SMTP_PORT", "587"))
	m := mailerpkg.New(mailerpkg.Config{
		ResendAPIKey: os.Getenv("RESEND_API_KEY"),
		Host:         os.Getenv("SMTP_HOST"),
		Port:         port,
		Username:     os.Getenv("SMTP_USERNAME"),
		Password:     os.Getenv("SMTP_PASSWORD"),
		From:         mustEnv("SMTP_FROM"),
		TLS:          envOr("SMTP_TLS", "false") == "true",
	})

	to := mustEnv("SMTP_FROM") // send to yourself
	fmt.Printf("Sending test email to %s ...\n", to)

	if err := m.NotifyNewLogin(context.Background(), to, "Test Device (macOS)", "127.0.0.1"); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK — check your inbox!")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "missing env var: %s\n", key)
		os.Exit(1)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
