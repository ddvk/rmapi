package api

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/juruen/rmapi/api/sync15"
	"github.com/juruen/rmapi/filetree"
	"github.com/juruen/rmapi/model"
	"github.com/juruen/rmapi/transport"
)

type ApiCtx interface {
	Filetree() *filetree.FileTreeCtx
	FetchDocument(docId, dstPath string) error
	CreateDir(parentId, name string, notify bool) (*model.Document, error)
	UploadDocument(parentId string, sourceDocPath string, notify bool, coverpage *int) (*model.Document, error)
	ReplaceDocumentFile(docId, sourceDocPath string, notify bool) error
	MoveEntry(src, dstDir *model.Node, name string) (*model.Node, error)
	DeleteEntry(node *model.Node, recursive, notify bool) error
	SyncComplete() error
	Nuke() error
	Refresh() (string, int64, error)
}

type UserToken struct {
	Auth0 struct {
		UserID string
		Email  string
	} `json:"auth0-profile"`
	Scopes string
	*jwt.StandardClaims
}

type SyncVersion int

const (
	Version15 SyncVersion = 15
)

func (s SyncVersion) String() string {
	switch s {
	case Version15:
		return "1.5"
	default:
		return "unknown"
	}
}

type UserInfo struct {
	SyncVersion SyncVersion
	User        string
}

func ParseToken(userToken string) (token *UserInfo, err error) {
	claims := UserToken{}
	_, _, err = (&jwt.Parser{}).ParseUnverified(userToken, &claims)

	if err != nil {
		return nil, fmt.Errorf("can't parse token %v", err)
	}

	if !claims.VerifyExpiresAt(time.Now().Unix(), false) {
		return nil, errors.New("token Expired")
	}

	token = &UserInfo{
		User:        claims.Auth0.Email,
		SyncVersion: Version15,
	}

	scopes := strings.Fields(claims.Scopes)

	for _, scope := range scopes {
		switch scope {
		case "sync:fox", "sync:tortoise", "sync:hare":
			token.SyncVersion = Version15
			return
		}
	}
	return token, nil
}

// CreateApiCtx initializes an instance of ApiCtx
func CreateApiCtx(httpCtx *transport.HttpClientCtx, syncVerison SyncVersion) (ctx ApiCtx, err error) {
	switch syncVerison {
	case Version15:
		return sync15.CreateCtx(httpCtx)
	default:
		log.Fatal("Unsupported sync version")
	}
	return
}
