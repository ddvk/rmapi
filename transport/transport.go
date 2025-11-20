package transport

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/juruen/rmapi/log"
	"github.com/juruen/rmapi/model"
	"github.com/juruen/rmapi/util"
)

type AuthType int

type BodyString struct {
	Content string
}

var ErrUnauthorized = errors.New("401 Unauthorized")
var ErrConflict = errors.New("409 Conflict")
var ErrWrongGeneration = errors.New("412 wrong generation")
var ErrNotFound = errors.New("not found")

var RmapiUserAGent = "rmapi"

const (
	EmptyBearer AuthType = iota
	DeviceBearer
	UserBearer
)

const (
	EmptyBody string = ""
)

type HttpClientCtx struct {
	Client *http.Client
	Tokens model.AuthTokens
}

func CreateHttpClientCtx(tokens model.AuthTokens) HttpClientCtx {
	var httpClient = &http.Client{Timeout: 5 * 60 * time.Second}

	return HttpClientCtx{httpClient, tokens}
}

func (ctx HttpClientCtx) addAuthorization(req *http.Request, authType AuthType) {
	var header string

	switch authType {
	case EmptyBearer:
		header = "Bearer"
	case DeviceBearer:
		header = fmt.Sprintf("Bearer %s", ctx.Tokens.DeviceToken)
	case UserBearer:
		header = fmt.Sprintf("Bearer %s", ctx.Tokens.UserToken)
	}

	req.Header.Add("Authorization", header)
}

func (ctx HttpClientCtx) Get(authType AuthType, url string, body interface{}, target interface{}) error {
	bodyReader, err := util.ToIOReader(body)

	if err != nil {
		log.Error.Println("failed to serialize body", err)
		return err
	}

	response, err := ctx.Request(authType, http.MethodGet, url, bodyReader, nil, 0)

	if response != nil {
		defer response.Body.Close()
	}

	if err != nil {
		return err
	}

	return json.NewDecoder(response.Body).Decode(target)
}

const RmFileNameHeader = "rm-filename"

func (ctx HttpClientCtx) GetStream(authType AuthType, url string, name string) (io.ReadCloser, error) {
	headers := map[string]string{
		RmFileNameHeader: name,
	}
	response, err := ctx.Request(authType, http.MethodGet, url, strings.NewReader(""), headers, 0)
	if err != nil {
		return nil, err
	}
	return response.Body, err
}

func (ctx HttpClientCtx) Post(authType AuthType, url string, reqBody, resp interface{}) error {
	return ctx.httpRawReq(authType, http.MethodPost, url, reqBody, resp, nil)
}

func (ctx HttpClientCtx) Put(authType AuthType, url string, reqBody, resp interface{}, headers map[string]string) error {
	return ctx.httpRawReq(authType, http.MethodPut, url, reqBody, resp, headers)
}

func (ctx HttpClientCtx) PutStream(authType AuthType, url string, reqBody io.Reader, name string, extraHeaders map[string]string) error {
	headers := map[string]string{
		RmFileNameHeader: name,
	}
	for k, v := range extraHeaders {
		headers[k] = v
	}
	return ctx.httpRawReq(authType, http.MethodPut, url, reqBody, nil, headers)
}

func (ctx HttpClientCtx) Delete(authType AuthType, url string, reqBody, resp interface{}) error {
	return ctx.httpRawReq(authType, http.MethodDelete, url, reqBody, resp, nil)
}

var table = crc32.MakeTable(crc32.Castagnoli)

func generateCRC32CFromReader(reader io.Reader) (string, error) {
	// Create a table for CRC32C (Castagnoli polynomial)
	// Create a CRC32C hasher
	crc32c := crc32.New(table)

	// Copy the reader data into the hasher
	if _, err := io.Copy(crc32c, reader); err != nil {
		return "", err
	}

	// Compute the CRC32C checksum
	checksum := crc32c.Sum32()

	// Convert the checksum to a byte array
	crcBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(crcBytes, checksum)
	encodedChecksum := base64.StdEncoding.EncodeToString(crcBytes)

	return encodedChecksum, nil
}

func (ctx HttpClientCtx) httpRawReq(authType AuthType, verb, url string, reqBody, resp interface{}, headers map[string]string) error {
	var contentBody io.Reader

	if headers == nil {
		headers = map[string]string{}
	}

	switch reqBody.(type) {
	case io.Reader:
		contentBody = reqBody.(io.Reader)
	default:
		c, err := util.ToIOReader(reqBody)

		if err != nil {
			log.Error.Println("failed to serialize body", err)
			return nil
		}

		contentBody = c
	}

	// calculate crc32c for google and set length
	var length int64
	if seeker, ok := contentBody.(io.ReadSeeker); ok {
		crc, err := generateCRC32CFromReader(seeker)
		if err != nil {
			return err
		}
		headers["x-goog-hash"] = "crc32c=" + crc
		length, err = seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("cannot get content length")
		}
		if _, exists := headers["content-type"]; !exists {
			headers["content-type"] = "application/octet-stream"
		}
		// headers["Content-Length"] = strconv.FormatInt(length, 10)
		_, err = seeker.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("cannot seek %v", err)
		}
	}
	response, err := ctx.Request(authType, verb, url, contentBody, headers, length)

	if response != nil {
		defer response.Body.Close()
	}

	if err != nil {
		return err
	}

	// We want to ingore the response
	if resp == nil {
		return nil
	}

	switch resp.(type) {
	case *BodyString:
		bodyContent, err := io.ReadAll(response.Body)

		if err != nil {
			return err
		}

		resp.(*BodyString).Content = string(bodyContent)
	default:
		err := json.NewDecoder(response.Body).Decode(resp)

		if err != nil {
			log.Error.Println("failed to deserialize body", err, response.Body)
			return err
		}
	}
	return nil
}

func (ctx HttpClientCtx) Request(authType AuthType, verb, url string, body io.Reader, headers map[string]string, length int64) (*http.Response, error) {
	request, err := http.NewRequest(verb, url, body)
	if err != nil {
		return nil, err
	}

	ctx.addAuthorization(request, authType)
	request.Header["user-agent"] = []string{RmapiUserAGent}

	if headers != nil {
		for k, v := range headers {
			request.Header[k] = []string{v}
		}
	}

	log.Trace.Println("---- start request ---- ")
	request.ContentLength = length
	if log.TracingEnabled {
		withBody := true
		if length > 300 {
			withBody = false
		}
		drequest, err := httputil.DumpRequest(request, withBody)
		log.Trace.Printf("request: %s %v", string(drequest), err)
		if !withBody {
			fmt.Println("body not logged")
		}
	}

	response, err := ctx.Client.Do(request)

	if err != nil {
		log.Error.Println("http request failed with", err)
		return nil, err
	}

	if log.TracingEnabled {
		defer response.Body.Close()
		dresponse, err := httputil.DumpResponse(response, true)
		log.Trace.Printf("%s %v", string(dresponse), err)
	}

	if IsHTTPStatusOK(response.StatusCode) {
		log.Trace.Println("---- end request ----")
		return response, nil
	} else {
		log.Trace.Printf("request failed with status %d\n", response.StatusCode)
	}

	switch response.StatusCode {
	case http.StatusUnauthorized:
		return response, ErrUnauthorized
	case http.StatusConflict:
		return response, ErrConflict
	case http.StatusPreconditionFailed:
		return response, ErrWrongGeneration
	default:
		return response, fmt.Errorf("request failed with status %d", response.StatusCode)
	}
}

// IsHTTPStatusOK if the status is ok
func IsHTTPStatusOK(status int) bool {
	switch status {
	case http.StatusOK, http.StatusAccepted, http.StatusCreated:
		return true
	default:
		return false
	}
}
