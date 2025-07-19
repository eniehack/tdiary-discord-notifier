package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

type DiscordWebhookEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Timestamp   string `json:"timestamp"`
}

type DiscordWebhook struct {
	Content string                `json:"content"`
	Embeds  []DiscordWebhookEmbed `json:"embeds"`
}

const USERAGENT = "Mozilla/5.0 (compatible; tdiary-notifier/0.1; +https://github.com/eniehack/tdiary-notifier)"

func main() {
	rssURL := os.Getenv("RSS_URL")
	webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")

	if rssURL == "" || webhookURL == "" {
		log.Fatal("RSS_URL and DISCORD_WEBHOOK_URL environment variables are required")
	}

	// RSS取得
	feedparser := gofeed.NewParser()
	feedparser.UserAgent = USERAGENT
	feed, err := feedparser.ParseURL(rssURL)
	if err != nil {
		log.Fatalf("Failed to parse RSS: %v", err)
	}

	// 今日の日記をチェック
	today := time.Now().Format("2006-01-02")
	var todayEntries []*gofeed.Item

	for _, item := range feed.Items {
		diaryDate, err := extractDateFromURL(item.Link)
		if err != nil {
			log.Printf("Failed to extract date from URL %s: %v", item.Link, err)
			continue
		}

		if diaryDate.Format("2006-01-02") == today {
			todayEntries = append(todayEntries, item)
		}
	}

	if len(todayEntries) == 0 {
		log.Println("No diary entries for today")
		return
	}

	// Discord通知送信
	if err = sendDiscordNotification(webhookURL, todayEntries); err != nil {
		log.Fatalf("Failed to send Discord notification: %v", err)
	}
}

func extractDateFromURL(urlStr string) (time.Time, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return time.Time{}, err
	}
	dateParam := u.Query().Get("date")
	if dateParam == "" {
		return time.Time{}, fmt.Errorf("date parameter not found")
	}
	return time.Parse("20060102", dateParam)
}

func sendDiscordNotification(webhookUrl string, entries []*gofeed.Item) error {
	// 記事を箇条書きで作成
	var descriptions []string
	for _, entry := range entries {
		diaryDate, err := extractDateFromURL(entry.Link)
		if err != nil {
			log.Printf("Failed to extract date from URL %s: %v", entry.Link, err)
			// フォールバック: 日付なしで追加
			descriptions = append(descriptions, fmt.Sprintf("- [%s](%s)", entry.Title, entry.Link))
		} else {
			dateStr := diaryDate.Format("2006年1月2日")
			descriptions = append(descriptions, fmt.Sprintf("- [%s - %s](%s)", dateStr, entry.Title, entry.Link))
		}
	}

	description := strings.Join(descriptions, "\n")

	// 文字数制限チェック（4096文字まで）
	if len(description) > 4000 {
		description = description[:3997] + "..."
	}

	webhook := &DiscordWebhook{
		Content: "📝 今日更新の日記まとめ",
		Embeds: []DiscordWebhookEmbed{
			{
				Title:       fmt.Sprintf("%s の日記 (%d件)", time.Now().Format("2006年01月02日"), len(entries)),
				Description: description,
				Timestamp:   time.Now().Format(time.RFC3339),
			},
		},
	}

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(webhook); err != nil {
		return fmt.Errorf("failed to marshal webhook data: %w", err)
	}

	client := new(http.Client)
	client.Timeout = time.Second * 30
	req, _ := http.NewRequest("POST", webhookUrl, buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", USERAGENT)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		buf := new(bytes.Buffer)
		io.Copy(buf, resp.Body)
		return fmt.Errorf("webhook failed with status %d: %s", resp.StatusCode, buf.String())
	}

	return nil
}
