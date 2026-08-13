package classify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

type Result struct {
	Title    string `json:"title"`
	Year     string `json:"year"`
	Kind     string `json:"kind"`     // movie / tv
	Category string `json:"category"` // 电影 / 国剧 / 日韩剧 / 欧美剧 / 动漫 / 综艺 / 未分类
	Season   int    `json:"season,omitempty"`
	Source   string `json:"source"` // tmdb / ai / heuristic
}

type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func New(baseURL, apiKey, model, proxy string) *Client {
	if model == "" {
		model = "gpt-4o-mini"
	}
	transport := &http.Transport{}
	if strings.TrimSpace(proxy) != "" {
		if u, err := url.Parse(proxy); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Transport: transport, Timeout: 25 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.apiKey != "" && c.baseURL != ""
}

func (c *Client) Ping(ctx context.Context) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("ai 未配置")
	}
	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "user", "content": "只回复两个字母：ok"},
		},
		"temperature": 0,
		"max_tokens":  8,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	url := c.baseURL
	if !strings.HasSuffix(url, "/chat/completions") {
		url = strings.TrimRight(url, "/") + "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ai status %d: %s", resp.StatusCode, truncate(string(b), 160))
	}
	var wrap struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return "", err
	}
	if len(wrap.Choices) == 0 {
		return "", fmt.Errorf("ai 无返回")
	}
	return strings.TrimSpace(wrap.Choices[0].Message.Content), nil
}

func (c *Client) Classify(ctx context.Context, query, title, hintCat string) (*Result, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("ai 未配置")
	}
	prompt := fmt.Sprintf(`根据影视资源信息判断分类并给出规范中文片名。
搜索词: %s
资源标题: %s
来源分类提示: %s
只返回 JSON，不要其它文字，字段：
{"title":"规范中文片名","year":"年份或空","kind":"movie或tv","category":"电影|国剧|日韩剧|欧美剧|动漫|综艺|未分类","season":季数整数没有则1}`, query, title, hintCat)

	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是影视资料整理助手，只输出合法 JSON。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.1,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := c.baseURL
	if !strings.HasSuffix(url, "/chat/completions") {
		url = strings.TrimRight(url, "/") + "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ai status %d: %s", resp.StatusCode, truncate(string(b), 200))
	}
	var wrap struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return nil, err
	}
	if len(wrap.Choices) == 0 {
		return nil, fmt.Errorf("ai 无返回")
	}
	content := strings.TrimSpace(wrap.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	var out Result
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("解析 AI JSON 失败: %w", err)
	}
	out.Source = "ai"
	if out.Kind != "tv" {
		out.Kind = "movie"
	}
	if out.Season <= 0 {
		out.Season = 1
	}
	out.Category = normalizeCategory(out.Category, out.Kind)
	return &out, nil
}

func Heuristic(query, title, hintCat string) Result {
	text := title + " " + query + " " + hintCat
	low := strings.ToLower(text)
	kind := "movie"
	cat := "电影"
	if strings.Contains(low, "s0") || strings.Contains(low, "season") ||
		strings.Contains(text, "第") && (strings.Contains(text, "季") || strings.Contains(text, "集")) ||
		hintCat == "dlz" || hintCat == "rj" || hintCat == "mj" || hintCat == "zy" || hintCat == "tv" {
		kind = "tv"
		cat = "欧美剧"
		if hasCJK(query) || hasCJK(title) {
			cat = "国剧"
		}
		if strings.Contains(text, "日") || strings.Contains(text, "韩") || strings.Contains(low, "jp") || strings.Contains(low, "kr") {
			cat = "日韩剧"
		}
		if hintCat == "zy" || strings.Contains(text, "综艺") {
			cat = "综艺"
		}
	}
	if strings.Contains(text, "动漫") || hintCat == "zydy" || hintCat == "jddy" {
		cat = "动漫"
		if strings.Contains(text, "剧") {
			kind = "tv"
		}
	}
	name := query
	if name == "" {
		name = title
	}
	if i := strings.Index(name, "|"); i > 0 {
		name = strings.TrimSpace(name[:i])
	}
	return Result{Title: strings.TrimSpace(name), Kind: kind, Category: cat, Season: 1, Source: "heuristic"}
}

func normalizeCategory(cat, kind string) string {
	switch cat {
	case "电影", "国剧", "日韩剧", "欧美剧", "动漫", "综艺", "未分类":
		return cat
	}
	if kind == "tv" {
		return "欧美剧"
	}
	return "电影"
}

func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
