package sync15

import (
	"fmt"
	"io"

	"github.com/juruen/rmapi/config"
	"github.com/juruen/rmapi/log"
	"github.com/juruen/rmapi/model"
	"github.com/juruen/rmapi/transport"
)

type BlobStorage struct {
	http        *transport.HttpClientCtx
	concurrency int
}

func NewBlobStorage(http *transport.HttpClientCtx) *BlobStorage {
	return &BlobStorage{
		http: http,
	}
}

func (b *BlobStorage) GetReader(hash, filename string) (io.ReadCloser, error) {
	return b.http.GetStream(transport.UserBearer, config.BlobUrl+hash, filename)
}

func (b *BlobStorage) UploadBlob(hash, filename string, reader io.Reader) error {
	log.Trace.Println("uploading blob ", filename)

	headers := map[string]string{}
	if filename == "root.docSchema" {
		headers["content-type"] = "text/plain; charset=UTF-8"
	}
	return b.http.PutStream(transport.UserBearer, config.BlobUrl+hash, reader, filename, headers)
}

// SyncComplete no longer used
func (b *BlobStorage) SyncComplete(gen int64) error {
	return nil
}

func (b *BlobStorage) WriteRootIndex(roothash string, gen int64, notify bool) (int64, error) {
	log.Info.Println("writing root with gen: ", gen)
	req := model.BlobRootStorageRequest{
		Broadcast:  notify,
		Hash:       roothash,
		Generation: gen,
	}
	var res model.BlobRootStorageResponse
	headers := map[string]string{
		transport.RmFileNameHeader: "roothash",
	}

	err := b.http.Put(transport.UserBearer, config.RootPut, req, &res, headers)
	if err != nil {
		return 0, err
	}
	if res.Hash != roothash {
		return 0, fmt.Errorf("bug? root hash mismatch")
	}

	return res.Generation, nil
}
func (b *BlobStorage) GetRootIndex() (string, int64, error) {
	var res model.BlobRootStorageResponse
	err := b.http.Get(transport.UserBearer, config.RootGet, nil, &res)
	if err != nil {
		return "", 0, err
	}

	log.Info.Println("got root gen:", res.Generation)
	return res.Hash, res.Generation, nil

}
