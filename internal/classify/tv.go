package classify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

type EpisodeGuess struct {
	Old    string `json:"old"`
	Season int    `json:"season"`
	Ep     int    `json:"ep"`
}

// GuessEpisodes 用 AI 从剧集文件名解析季/集；解析不到的文件不返回。
func (c *Client) GuessEpisodes(ctx context.Context, showTitle string, seasonHint int, names []string) ([]EpisodeGuess, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("ai 未配置")
	}
	if len(names) == 0 {
		return nil, nil
	}
	if seasonHint <= 0 {
		seasonHint = 1
	}
	var b strings.Builder
	b.WriteString("剧名: ")
	b.WriteString(showTitle)
	b.WriteString(fmt.Sprintf("\n目录季数提示: %d\n文件名列表:\n", seasonHint))
	for i, n := range names {
		if i >= 40 {
			break
		}
		b.WriteString("- ")
		b.WriteString(n)
		b.WriteByte('\n')
	}
	b.WriteString(`只返回 JSON 数组，不要其它文字。每项: {"old":"原文件名含扩展名","season":季数,"ep":集数}
无法判断的文件不要列入。season 没有则用提示季数。`)

	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是剧集文件名解析助手，只输出合法 JSON 数组。"},
			{"role": "user", "content": b.String()},
		},
		"temperature": 0,
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
	rawResp, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ai status %d: %s", resp.StatusCode, truncate(string(rawResp), 200))
	}
	var wrap struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rawResp, &wrap); err != nil {
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
	var out []EpisodeGuess
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("解析 AI 剧集 JSON 失败: %w", err)
	}
	seen := map[string]bool{}
	clean := make([]EpisodeGuess, 0, len(out))
	for _, g := range out {
		g.Old = strings.TrimSpace(g.Old)
		if g.Old == "" || g.Ep <= 0 {
			continue
		}
		if g.Season <= 0 {
			g.Season = seasonHint
		}
		key := path.Base(g.Old)
		if seen[key] {
			continue
		}
		seen[key] = true
		g.Old = key
		clean = append(clean, g)
	}
	return clean, nil
}
