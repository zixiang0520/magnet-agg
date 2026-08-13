package drive

import (
	"context"
	"fmt"
	"log"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/halalcloud/golang-sdk-lite/halalcloud/services/userfile"
)

var videoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".rmvb": true,
	".ts": true, ".m4v": true, ".mov": true, ".wmv": true,
	".flv": true, ".webm": true, ".iso": true, ".mpg": true,
	".mpeg": true, ".3gp": true, ".vob": true, ".m2ts": true,
	".mts": true, ".f4v": true, ".rm": true, ".asf": true,
}

var epRe = regexp.MustCompile(`(?i)(?:S\d{1,2})?E(?:P)?(\d{1,3})\b`)
var epCnRe = regexp.MustCompile(`第(\d{1,3})集`)
var seasonNumRe = regexp.MustCompile(`第(\d{1,2})季`)
var yearSuffixRe = regexp.MustCompile(`\s*\(\d{4}\)\s*$`)

type OrganizeResult struct {
	SavePath string         `json:"save_path"`
	Category string         `json:"category"`
	IsTV     bool           `json:"is_tv"`
	TitleDir string         `json:"title_dir"`
	Season   int            `json:"season"`
	Deleted  []string       `json:"deleted"`
	Renamed  []RenameRecord `json:"renamed"`
	Skipped  []string       `json:"skipped"`
	AIUsed   bool           `json:"ai_used"`
}

type RenameRecord struct {
	Old string `json:"old"`
	New string `json:"new"`
}

func (c *Client) OrganizeTask(ctx context.Context, savePath string) (*OrganizeResult, error) {
	savePath = strings.TrimRight(savePath, "/")
	if savePath == "" {
		savePath = "/"
	}
	res := &OrganizeResult{SavePath: savePath}
	cat := categoryFromSavePath(savePath)
	res.Category = cat
	res.IsTV = isTVCategory(cat)
	res.Season = 1
	parts := strings.Split(strings.TrimPrefix(savePath, "/"), "/")
	if len(parts) >= 3 {
		res.TitleDir = parts[2]
	}
	if res.IsTV && len(parts) >= 4 {
		if m := seasonNumRe.FindStringSubmatch(parts[3]); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				res.Season = n
			}
		}
	}

	allFiles, err := c.listAllFilesRecursive(ctx, savePath)
	if err != nil {
		return nil, fmt.Errorf("列出文件失败: %w", err)
	}
	if len(allFiles) == 0 {
		return res, nil
	}

	var adIDs, adNames []string
	var videos []*userfile.File
	for _, f := range allFiles {
		if f.Dir {
			continue
		}
		ext := strings.ToLower(path.Ext(f.Name))
		if videoExts[ext] {
			videos = append(videos, f)
		} else {
			adIDs = append(adIDs, f.Identity)
			adNames = append(adNames, f.Name)
		}
	}
	if len(adIDs) > 0 {
		if err := c.DeleteFiles(ctx, adIDs); err != nil {
			log.Printf("OrganizeTask: delete ads failed: %v", err)
		} else {
			res.Deleted = adNames
		}
	}

	aiMap := map[string][2]int{}
	if res.IsTV {
		needAI := false
		for _, v := range videos {
			if parseEpisode(v.Name) == 0 {
				needAI = true
				break
			}
		}
		s := c.snap()
		if needAI && s.ai != nil && s.ai.Enabled() {
			names := make([]string, 0, len(videos))
			for _, v := range videos {
				names = append(names, v.Name)
			}
			if guesses, err := s.ai.GuessEpisodes(ctx, stripYear(res.TitleDir), res.Season, names); err == nil {
				res.AIUsed = true
				for _, g := range guesses {
					aiMap[g.Old] = [2]int{g.Season, g.Ep}
				}
			} else {
				log.Printf("OrganizeTask: AI episode guess failed: %v", err)
			}
		}
	}

	movieIdx := 0
	pureTitle := stripYear(res.TitleDir)
	for _, v := range videos {
		ext := path.Ext(v.Name)
		var newName string
		if res.IsTV {
			ep := parseEpisode(v.Name)
			season := res.Season
			if ep == 0 {
				if g, ok := aiMap[v.Name]; ok {
					season, ep = g[0], g[1]
				}
			}
			if ep == 0 {
				res.Skipped = append(res.Skipped, v.Name)
				continue
			}
			if pureTitle == "" {
				newName = fmt.Sprintf("S%02dE%02d%s", season, ep, ext)
			} else {
				newName = fmt.Sprintf("%s S%02dE%02d%s", pureTitle, season, ep, ext)
			}
		} else {
			if res.TitleDir == "" {
				res.Skipped = append(res.Skipped, v.Name)
				continue
			}
			movieIdx++
			if movieIdx == 1 {
				newName = res.TitleDir + ext
			} else {
				newName = fmt.Sprintf("%s-%d%s", res.TitleDir, movieIdx, ext)
			}
		}
		if newName == v.Name {
			continue
		}
		if err := c.Rename(ctx, v.Identity, newName); err != nil {
			log.Printf("OrganizeTask: rename %q -> %q failed: %v", v.Name, newName, err)
			res.Skipped = append(res.Skipped, v.Name+" (重命名失败)")
			continue
		}
		res.Renamed = append(res.Renamed, RenameRecord{Old: v.Name, New: newName})
	}

	savePathTrim := strings.TrimRight(savePath, "/")
	var moveIDs []string
	for _, v := range videos {
		if path.Dir(v.Path) != savePathTrim {
			moveIDs = append(moveIDs, v.Identity)
		}
	}
	if len(moveIDs) > 0 {
		if err := c.Move(ctx, moveIDs, savePath); err != nil {
			log.Printf("OrganizeTask: move failed: %v", err)
		}
	}

	topFiles, err := c.ListFiles(ctx, savePath)
	if err == nil {
		var dirIDs []string
		for _, f := range topFiles {
			if !f.Dir {
				continue
			}
			subs, subErr := c.listAllFilesRecursive(ctx, f.Path)
			if subErr != nil || len(subs) > 0 {
				continue
			}
			dirIDs = append(dirIDs, f.Identity)
		}
		if len(dirIDs) > 0 {
			_ = c.DeleteFiles(ctx, dirIDs)
		}
	}
	return res, nil
}

func categoryFromSavePath(savePath string) string {
	parts := strings.Split(strings.TrimPrefix(strings.TrimRight(savePath, "/"), "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func (c *Client) listAllFilesRecursive(ctx context.Context, parentPath string) ([]*userfile.File, error) {
	files, err := c.ListFiles(ctx, parentPath)
	if err != nil {
		return nil, err
	}
	var all []*userfile.File
	for _, f := range files {
		if f.Dir {
			subs, err := c.listAllFilesRecursive(ctx, f.Path)
			if err != nil {
				continue
			}
			all = append(all, subs...)
		} else {
			all = append(all, f)
		}
	}
	return all, nil
}

func parseEpisode(name string) int {
	base := strings.TrimSuffix(name, path.Ext(name))
	if m := epRe.FindStringSubmatch(base); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return n
		}
	}
	if m := epCnRe.FindStringSubmatch(base); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func stripYear(titleDir string) string {
	return strings.TrimSpace(yearSuffixRe.ReplaceAllString(titleDir, ""))
}
