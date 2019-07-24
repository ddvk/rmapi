package shell

import (
	"errors"
	"encoding/json"
	"net/http"
	"fmt"
	"os"
	"time"
	"sort"
	"io/ioutil"
	
	"github.com/abiosoft/ishell"
	//"github.com/peterhellberg/link"
	//"github.com/peerdavid/rmapi/filetree"
	//"github.com/peerdavid/rmapi/model"
)

var UserId string = os.Getenv("ZOTERO_USERID")
var ApiKey string = os.Getenv("ZOTERO_APIKEY")
var myClient = &http.Client{Timeout: 10 * time.Second}
const BaseZoteroURL string = "https://api.zotero.org/users/"


type ZoteroItem struct {
	Key  string         `json:"key"`
	Data ZoteroItemData `json:"data"`
	//Links interface{}	`json:"links"`
}

type ZoteroItemData struct {
	ContentType string `json:"contentType"`
	Filename    string `json:"filename"`
	Url         string `json:"url"`
}

type ZoteroDirectory struct {
	Key  string        `json:"key"`
	Data ZoteroDirData `json:"data"`
}

type ZoteroDirData struct {
	Key              string      `json:"key"`
	Version          int         `json:"version"`
	Name             string      `json:"name"`
	ParentCollection interface{} `json:"parentCollection"`
	Relations        interface{} `json:"relations"`
}

func getJson(url string, target interface{}) (*http.Response, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Zotero-API-Key", ApiKey)
	res, err := myClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	return res, json.NewDecoder(res.Body).Decode(target)
}

func getZoteroDirectories() ([]ZoteroDirectory, error) {
	var directoriesJson []ZoteroDirectory
	_, err := getJson(BaseZoteroURL + UserId + "/collections", &directoriesJson)
	if err != nil {
		return nil, err
	}
	return directoriesJson, nil
}

func getZoteroItemsForDirectory(directory ZoteroDirectory) ([]ZoteroItem, error) {
	var zoteroItems []ZoteroItem
	_, err := getJson(BaseZoteroURL+UserId+"/collections/"+directory.Key+"/items", &zoteroItems)
	if err != nil {
		return nil, err
	}
	return zoteroItems, nil
}

func getFileFromZotero(item ZoteroItem) ([]byte, error) {
	url := BaseZoteroURL+UserId+"/items/"+item.Key+"/file"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Zotero-API-Key", ApiKey)
	res, err := myClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}

		//ioutil.WriteFile(item.Data.Filename, bodyBytes, 0644)
		//bodyString := string(bodyBytes)
		//fmt.Println(bodyString)
		return bodyBytes, nil
	}

	return nil, errors.New("Http error while download zotero pdf file")
}


func printFromZotero(url string) () {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Zotero-API-Key", ApiKey)
	res, err := myClient.Do(req)
	if err != nil {
		fmt.Println(err)
		return 
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(res.Body)
		if err != nil {
			fmt.Println(err)
			return 
		}
		bodyString := string(bodyBytes)
		fmt.Println(bodyString)
	}
}


func getFullZoteroPath(directory ZoteroDirectory, dirMap map[string]ZoteroDirectory, direcotries []ZoteroDirectory) (string){
	tmp := directory.Data.Name
	for directory.Data.ParentCollection != false {
		parentId := fmt.Sprintf("%v", directory.Data.ParentCollection)
		directory = dirMap[parentId]
		tmp = directory.Data.Name + "/" + tmp
	}
	return tmp
}


func zoteroCmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "zotero",
		Help:      "sync zotero cloud to remarkable cloud",
		Completer: createDirCompleter(ctx),
		Func: func(c *ishell.Context) {
			if len(c.Args) == 0 {
				c.Err(errors.New(("missing source dir")))
				return
			}

			rmSrcDir := c.Args[0] + "/"
			node, err := ctx.api.Filetree.NodeByPath(rmSrcDir, ctx.node)

			if err != nil || node.IsFile() {
				c.Err(errors.New("remote directory does not exist"))
				return
			}

			path, err := ctx.api.Filetree.NodeToPath(node)
			if err != nil || node.IsFile() {
				c.Err(errors.New("remote directory does not exist"))
				return
			}


			if err != nil || node.IsFile() {
				c.Err(errors.New("zotero directory doesn't exist on rm cloud"))
				return
			}
			
			// ToDo: Read all zotero files
			zoteroDirectories, err := getZoteroDirectories()
			if err != nil {
				fmt.Println(err)
				c.Err(errors.New("failed to connect to zotero"))
				return
			}

			// Create dirmap (id, dirname) and fulldirmap (id, path)
			var dirMap map[string]ZoteroDirectory
			dirMap = make(map[string]ZoteroDirectory)
			for _, zoteroDir := range zoteroDirectories{
				dirMap[zoteroDir.Key] = zoteroDir
			}

			var fullDirMap map[string]string
			fullDirMap = make(map[string]string)
			for _, zoteroDir := range zoteroDirectories{
				fullDir := getFullZoteroPath(zoteroDir, dirMap, zoteroDirectories)
				fullDirMap[zoteroDir.Key] = fullDir
			}

			keys := make([]string, 0, len(fullDirMap))
			for k := range fullDirMap {
				keys = append(keys, k)
			}

			sort.SliceStable(keys, func(i, j int) bool {
				return len(fullDirMap[keys[i]]) < len(fullDirMap[keys[j]])
			})
		
			//url := BaseZoteroURL+UserId+"/collections"
			//printFromZotero(url)

			// Download all zotero files
			// ToDo: Download only if file does not exist in rm cloud
			hiddenFolder := ".tmpZoteroRmSync/"
			os.MkdirAll(hiddenFolder, os.ModePerm)
			
			for _, key := range keys{
				zoteroDir := dirMap[key]
				path := fullDirMap[zoteroDir.Key]
				zoteroPath := hiddenFolder + path

				// Create local folder with files
				os.MkdirAll(zoteroPath, os.ModePerm)
				items, err := getZoteroItemsForDirectory(zoteroDir)
				if err != nil {
					c.Err(errors.New("failed to read items from zotero folder"))
					return
				}
				
				for _, item := range items{
					if item.Data.ContentType == "application/pdf"{
						rmPath := rmSrcDir + path + "/" + item.Data.Filename
						_, err := ctx.api.Filetree.NodeByPath(rmPath, ctx.node)
						if err == nil{
							fmt.Println(rmPath + "...already exists on rm." )
							continue
						}

						fmt.Println(rmPath + " ...download from zotero." )
						file, err := getFileFromZotero(item)
						if err != nil {
							c.Err(errors.New("failed to download pdf " + item.Data.Filename + " from zotero"))
						}
						ioutil.WriteFile(zoteroPath + "/" + item.Data.Filename, file, 0644)
					}
				}
			}

			// Upload to rm
			fmt.Println("Upload all new papers to rm cloud..." )
			treeFormatStr := "├"

			// Back up current remote location.
			currCtxPath := ctx.path
			currCtxNode := ctx.node
			// Change to requested directory.
			ctx.path = path
			ctx.node = node

			c.Println()
			putFilesAndDirs(ctx, c, "./" + hiddenFolder, 0, &treeFormatStr)
			c.Println()

			// Reset.
			ctx.path = currCtxPath
			ctx.node = currCtxNode

			// Delete hidden folder
			os.RemoveAll(hiddenFolder)
		},
	}
}
