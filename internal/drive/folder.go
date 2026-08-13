package drive

import (
	"context"
	"log"
	"strings"

	"github.com/halalcloud/golang-sdk-lite/halalcloud/services/userfile"
)

var categoryNames = map[string]string{
	"dy": "电影", "gydy": "国语电影", "gq": "经典高清",
	"zydy": "动漫", "jddy": "动画电影", "3D": "3D电影",
	"dlz": "国剧", "rj": "日韩剧", "mj": "欧美剧",
	"zy": "综艺", "shoujidianyingmp4": "手机电影",
	"movie": "电影", "tv": "剧集",
}

var tvCategories = map[string]bool{
	"dlz": true, "rj": true, "mj": true, "zy": true, "tv": true,
	"国剧": true, "日韩剧": true, "欧美剧": true, "综艺": true, "剧集": true,
}

func categoryName(cat string) string {
	if n, ok := categoryNames[cat]; ok {
		return n
	}
	switch cat {
	case "电影", "国剧", "日韩剧", "欧美剧", "动漫", "综艺", "未分类", "剧集":
		return cat
	}
	if cat == "" {
		return "未分类"
	}
	return cat
}

func isTVCategory(cat string) bool { return tvCategories[cat] || tvCategories[categoryName(cat)] }

func sanitize(name string) string {
	r := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	s := strings.TrimSpace(r.Replace(name))
	if s == "" {
		s = "未命名"
	}
	return s
}

func ensureFolderByCategory(ctx context.Context, s snapshot, category, titleName, seasonName string) (string, error) {
	log.Printf("ensureFolderByCategory: category=%q titleName=%q seasonName=%q baseDir=%q", category, titleName, seasonName, s.baseDir)
	base, baseID, err := ensureDir(ctx, s.userfile, "/", "", s.baseDir)
	if err != nil {
		return "", err
	}
	cat, catID, err := ensureDir(ctx, s.userfile, base, baseID, categoryName(category))
	if err != nil {
		return "", err
	}
	titlePath, titleID, err := ensureDir(ctx, s.userfile, cat, catID, titleName)
	if err != nil {
		return "", err
	}
	if isTVCategory(category) && seasonName != "" {
		sp, _, err := ensureDir(ctx, s.userfile, titlePath, titleID, seasonName)
		if err != nil {
			return "", err
		}
		return sp, nil
	}
	return titlePath, nil
}

func ensureDir(ctx context.Context, uf *userfile.UserFileService, parentPath, parentID, name string) (string, string, error) {
	if existing, _ := findDir(ctx, uf, parentPath, name); existing != nil {
		p := normalizePath(existing.Path, parentPath, name)
		return p, existing.Identity, nil
	}
	created, err := uf.Create(ctx, &userfile.File{
		Name:   name,
		Dir:    true,
		Parent: parentID,
	})
	if err != nil {
		return "", "", err
	}
	return normalizePath(created.Path, parentPath, name), created.Identity, nil
}

func normalizePath(apiPath, parentPath, name string) string {
	wantPath := joinPath(parentPath, name)
	if apiPath == "" || !strings.HasPrefix(apiPath, "/") {
		return wantPath
	}
	if parentPath == "/" {
		return apiPath
	}
	if apiPath == parentPath || strings.HasPrefix(apiPath, strings.TrimRight(parentPath, "/")+"/") {
		return apiPath
	}
	return wantPath
}

func findDir(ctx context.Context, uf *userfile.UserFileService, parentPath, name string) (*userfile.File, error) {
	resp, err := uf.List(ctx, &userfile.FileListRequest{
		Parent: &userfile.File{Path: parentPath},
	})
	if err != nil {
		return nil, err
	}
	for _, f := range resp.Files {
		if f.Dir && f.Name == name {
			return f, nil
		}
	}
	return nil, nil
}

func joinPath(parent, name string) string {
	if parent == "/" {
		return "/" + name
	}
	return strings.TrimRight(parent, "/") + "/" + name
}
