package drive

import (
	"context"
	"fmt"
	"log"

	"github.com/halalcloud/golang-sdk-lite/halalcloud/model"
	"github.com/halalcloud/golang-sdk-lite/halalcloud/services/userfile"
)

func (c *Client) ListFiles(ctx context.Context, parentPath string) ([]*userfile.File, error) {
	s := c.snap()
	const pageSize = 50
	var all []*userfile.File
	var token string
	for page := 0; ; page++ {
		resp, err := s.userfile.List(ctx, &userfile.FileListRequest{
			Parent:   &userfile.File{Path: parentPath},
			ListInfo: &model.ScanListRequest{Limit: pageSize, Token: token},
		})
		if err != nil {
			if page == 0 {
				return nil, err
			}
			log.Printf("ListFiles: page %d error: %v", page, err)
			break
		}
		all = append(all, resp.Files...)
		if resp.ListInfo == nil || resp.ListInfo.Token == "" || len(resp.Files) == 0 {
			break
		}
		token = resp.ListInfo.Token
		if page > 100 {
			break
		}
	}
	return all, nil
}

func (c *Client) Rename(ctx context.Context, identity, newName string) error {
	s := c.snap()
	_, err := s.userfile.Rename(ctx, &userfile.File{Identity: identity, Name: newName})
	return err
}

func (c *Client) Move(ctx context.Context, identities []string, destPath string) error {
	if len(identities) == 0 {
		return errEmptyIdentity
	}
	s := c.snap()
	dest, err := s.userfile.Get(ctx, &userfile.File{Path: destPath})
	if err != nil || dest == nil || dest.Identity == "" {
		return fmt.Errorf("移动目标目录 %q 不存在: %v", destPath, err)
	}
	srcs := make([]*userfile.File, 0, len(identities))
	for _, id := range identities {
		srcs = append(srcs, &userfile.File{Identity: id})
	}
	_, err = s.userfile.Move(ctx, &userfile.BatchOperationRequest{
		Source: srcs,
		Dest:   &userfile.File{Identity: dest.Identity},
	})
	return err
}

func (c *Client) DeleteFiles(ctx context.Context, identities []string) error {
	if len(identities) == 0 {
		return errEmptyIdentity
	}
	s := c.snap()
	srcs := make([]*userfile.File, 0, len(identities))
	for _, id := range identities {
		srcs = append(srcs, &userfile.File{Identity: id})
	}
	_, err := s.userfile.Trash(ctx, &userfile.BatchOperationRequest{Source: srcs})
	return err
}
