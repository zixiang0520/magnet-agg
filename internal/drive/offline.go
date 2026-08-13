package drive

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"magnet-agg/internal/classify"
	"magnet-agg/internal/tmdb"

	"github.com/halalcloud/golang-sdk-lite/halalcloud/model"
	"github.com/halalcloud/golang-sdk-lite/halalcloud/services/offline"
)

type PushItem struct {
	Name     string `json:"name"`
	Magnet   string `json:"magnet"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Query    string `json:"query,omitempty"`
}

type PushResultItem struct {
	Name     string `json:"name"`
	Magnet   string `json:"magnet"`
	Category string `json:"category"`
	Folder   string `json:"folder"`
	SavePath string `json:"save_path"`
	Season   string `json:"season,omitempty"`
	MetaFrom string `json:"meta_from,omitempty"`
	OK       bool   `json:"ok"`
	Identity string `json:"identity,omitempty"`
	Error    string `json:"error,omitempty"`
}

type PushResult struct {
	Items []PushResultItem `json:"items"`
}

func (c *Client) Push(ctx context.Context, items []PushItem) (*PushResult, error) {
	s := c.snap()
	res := &PushResult{}
	cache := map[string]string{}
	log.Printf("Push: start items=%d logged_in=%v baseDir=%q", len(items), c.LoggedIn(), s.baseDir)
	for i, it := range items {
		ri := PushResultItem{Name: it.Name, Magnet: it.Magnet, Category: it.Category}
		meta := resolveMeta(ctx, s, it)
		ri.MetaFrom = meta.Source
		ri.Category = meta.Category
		ri.Folder = meta.Folder
		log.Printf("Push[%d]: category=%q folder=%q kind=%s from=%s", i, meta.Category, meta.Folder, meta.Kind, meta.Source)

		seasonName := ""
		key := meta.Category + "|" + meta.Folder
		if meta.Kind == "tv" {
			n := meta.Season
			if n <= 0 {
				n = seasonFromMagnet(it.Magnet, it.Name+" "+it.Title)
			}
			seasonName = "第" + strconv.Itoa(n) + "季"
			ri.Season = seasonName
			key += "|" + seasonName
		}

		savePath, cached := cache[key]
		if !cached {
			sp, err := ensureFolderByCategory(ctx, s, meta.Category, meta.Folder, seasonName)
			if err != nil {
				ri.Error = "创建文件夹失败: " + err.Error()
				cache[key] = ""
				res.Items = append(res.Items, ri)
				continue
			}
			savePath = sp
			cache[key] = savePath
		}
		if savePath == "" {
			ri.Error = "创建文件夹失败"
			res.Items = append(res.Items, ri)
			continue
		}
		ri.SavePath = savePath
		task, err := s.offline.Add(ctx, &offline.UserTask{
			Url:      it.Magnet,
			Name:     it.Name,
			SavePath: savePath,
		})
		if err != nil {
			ri.Error = err.Error()
		} else {
			ri.OK = true
			ri.Identity = task.Identity
			c.startWatcher(task.Identity, savePath)
		}
		res.Items = append(res.Items, ri)
	}
	return res, nil
}

type resolvedMeta struct {
	Folder   string
	Category string
	Kind     string
	Season   int
	Source   string
}

func resolveMeta(ctx context.Context, s snapshot, it PushItem) resolvedMeta {
	query := strings.TrimSpace(it.Query)
	if query == "" {
		query = strings.TrimSpace(it.Title)
	}
	if query == "" {
		query = strings.TrimSpace(it.Name)
	}
	h := classify.Heuristic(query, it.Name, it.Category)
	preferTV := h.Kind == "tv" || isTVCategory(it.Category)

	var tmdbHit *tmdb.Result
	if s.tmdb != nil && query != "" {
		name, yearHint := parseTitle(query)
		if name == "" {
			name = query
		}
		if r, err := s.tmdb.SearchAuto(ctx, name, preferTV); err == nil && r != nil && r.Title != "" {
			tmdbHit = r
			_ = yearHint
		} else if name != query {
			if r, err := s.tmdb.SearchAuto(ctx, query, preferTV); err == nil && r != nil && r.Title != "" {
				tmdbHit = r
			}
		}
	}

	out := resolvedMeta{
		Folder:   sanitize(h.Title),
		Category: h.Category,
		Kind:     h.Kind,
		Season:   h.Season,
		Source:   "heuristic",
	}
	if tmdbHit != nil {
		y := tmdb.YearFromDate(tmdbHit.Date)
		folder := tmdbHit.Title
		if y != "" {
			folder = tmdbHit.Title + " (" + y + ")"
		}
		out.Folder = sanitize(folder)
		out.Kind = tmdbHit.MediaType
		if out.Kind == "tv" && !isTVCategory(out.Category) {
			out.Category = "欧美剧"
			if hasHan(tmdbHit.Title) || hasHan(query) {
				out.Category = "国剧"
			}
		}
		if out.Kind == "movie" {
			out.Category = "电影"
			if it.Category == "zydy" || it.Category == "jddy" {
				out.Category = "动漫"
			}
		}
		out.Source = "tmdb"
	}

	// 剧集：AI 辅助分类/季数（不覆盖 TMDB 片名）
	if out.Kind == "tv" && s.ai != nil && s.ai.Enabled() {
		if ai, err := s.ai.Classify(ctx, query, it.Name+" "+it.Title, it.Category); err == nil && ai != nil {
			if ai.Category != "" {
				out.Category = ai.Category
			}
			if ai.Season > 0 {
				out.Season = ai.Season
			}
			if out.Source == "tmdb" {
				out.Source = "tmdb+ai"
			} else {
				out.Folder = sanitize(folderOr(ai.Title, ai.Year, out.Folder))
				out.Source = "ai"
			}
		}
	}
	if out.Season <= 0 {
		out.Season = 1
	}
	return out
}

func folderOr(title, year, fallback string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return fallback
	}
	if year != "" {
		return sanitize(title + " (" + year + ")")
	}
	return sanitize(title)
}

var titleRe = regexp.MustCompile(`(?:([0-9]{4}))?[^\d《]*《([^》]+)》`)

func parseTitle(t string) (name, year string) {
	if m := titleRe.FindStringSubmatch(t); m != nil {
		return m[2], m[1]
	}
	return t, ""
}

func hasHan(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

func (c *Client) ListTasks(ctx context.Context) ([]*offline.UserTask, error) {
	s := c.snap()
	const pageSize = 50
	var all []*offline.UserTask
	var token string
	for page := 0; ; page++ {
		resp, err := s.offline.List(ctx, &offline.OfflineTaskListRequest{
			ListInfo: &model.ScanListRequest{Limit: pageSize, Token: token},
		})
		if err != nil {
			if page == 0 {
				return nil, err
			}
			break
		}
		all = append(all, resp.Tasks...)
		if resp.ListInfo == nil || resp.ListInfo.Token == "" || len(resp.Tasks) == 0 {
			break
		}
		token = resp.ListInfo.Token
		if page > 100 {
			break
		}
	}
	return all, nil
}

func (c *Client) DeleteTask(ctx context.Context, identities []string, deleteFiles bool) error {
	if len(identities) == 0 {
		return errEmptyIdentity
	}
	s := c.snap()
	body := map[string]any{
		"identity":     identities,
		"delete_files": deleteFiles,
	}
	result := struct {
		Count int64 `json:"count,string"`
	}{}
	return s.api.Post(ctx, "/v6/offline_task/delete", nil, body, &result)
}

var errEmptyIdentity = fmt.Errorf("identity 不能为空")
