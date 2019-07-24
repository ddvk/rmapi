package shell

import (
	"errors"
	"encoding/json"
	"net/http"
	"fmt"
	"os"
	"time"
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
	tmp := directory.Data.Name + "/"
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

			srcName := c.Args[0]

			node, err := ctx.api.Filetree.NodeByPath(srcName, ctx.node)

			if err != nil || node.IsFile() {
				c.Err(errors.New("directory doesn't exist"))
				return
			}
			
			// ToDo: Read all zotero files
			zoteroDirectories, err := getZoteroDirectories()
			if err != nil {
				fmt.Println(err)
				c.Err(errors.New("failed to connect to zotero"))
				return
			}

			// Create dirmap
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

			hiddenFolder := "tmpZoteroRmSync/"
			os.MkdirAll(hiddenFolder, os.ModePerm)
		
			//url := BaseZoteroURL+UserId+"/collections"
			//printFromZotero(url)

			// Download all zotero files
			// ToDo: Download only if file does not exist in rm cloud
			for _, zoteroDir := range zoteroDirectories{
				path := hiddenFolder + fullDirMap[zoteroDir.Key]
				os.MkdirAll(path, os.ModePerm)

				items, err := getZoteroItemsForDirectory(zoteroDir)
				if err != nil {
					c.Err(errors.New("failed to read items from zotero folder"))
					return
				}
				
				for _, item := range items{
					if item.Data.ContentType == "application/pdf"{
						file, err := getFileFromZotero(item)
						if err != nil {
							c.Err(errors.New("failed to download pdf from zotero"))
							return
						}
						fmt.Println(path + item.Data.Filename)
						ioutil.WriteFile(path + item.Data.Filename, file, 0644)
					}
				}
			}
		},
	}
}
