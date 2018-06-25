package main

import (
	"encoding/json"
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/gin-gonic/gin"
	"github.com/humboldt-xie/jsync"
	"github.com/vmware/harbor/src/common/utils/registry"
)

var jdata = jsync.Jsync{}
var jstatus = map[string]interface{}{}
var mu sync.Mutex

//TagStatus ...
type TagStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

//Project ...
type Project struct {
	Name    string       `json:"name"`
	Status  string       `json:"status"`
	Message string       `json:"message"`
	Tags    []*TagStatus `json:"tags"`
}

//Init ...
func (st *Project) Init() {
	st.Tags = []*TagStatus{}
}

//Get ...
/*func (st *Status) Get(tag string) *TagStatus {
	st.mu.Lock()
	defer st.mu.Unlock()
	status := st.Tags[tag]
	if status == nil {
		status = &TagStatus{}
		st.Tags[tag] = status
	}
	return status
}*/

//Replication ...
type Replication struct {
	Name     string
	Source   Registry
	Target   Registry
	Projects []*Project
	Filter   []string `json:"filter"`
	mu       sync.Mutex
}

//Init ...
func (rep *Replication) Init() error {
	rep.Target.Init()
	rep.Source.Init()
	rep.Projects = []*Project{}
	return nil
}

// GetStatus ...
func (rep *Replication) GetStatus(name string, tagName string) string {
	rep.mu.Lock()
	defer rep.mu.Unlock()
	var project *Project
	for _, project = range rep.Projects {
		if project.Name == name {
			break
		}
	}
	if project == nil {
		return ""
	}
	if tagName == "" {
		return project.Status
	}
	var tag *TagStatus
	for _, tag = range project.Tags {
		if tag.Name == tagName {
			break
		}
	}
	if tag == nil {
		return ""
	}
	return tag.Status
}

// SetStatus set a project status
func (rep *Replication) SetStatus(name string, tagName string, status string) {
	rep.mu.Lock()
	defer rep.mu.Unlock()
	var project *Project
	for _, project = range rep.Projects {
		if project.Name == name {
			break
		}
	}
	if project == nil {
		project = &Project{Name: name}
		project.Init()
		rep.Projects = append(rep.Projects, project)
	}
	if tagName == "" {
		project.Status = status
		return
	}
	var tag *TagStatus
	for _, tag = range project.Tags {
		if tag.Name == tagName {
			break
		}
	}
	if tag == nil {
		tag = &TagStatus{Name: tagName}
		project.Tags = append(project.Tags, tag)
	}
	if status == tag.Status {
		return
	}
	tag.Status = status
	mu.Lock()
	jstatus[rep.Name] = rep.Projects
	mu.Unlock()
	jdata.Update("status", jstatus)
}

//GetStatus ...
/*func (rep *Replication) GetStatus(r string) *Status {
	rep.mu.Lock()
	defer rep.mu.Unlock()
	status := rep.Status[r]
	if status == nil {
		status = &Status{mu: &rep.mu}
		status.Init()
		rep.Status[r] = status
	}
	return status
}*/

func (rep *Replication) pullManifest(reg *registry.Repository, tag string) (string, distribution.Manifest, error) {
	acceptMediaTypes := []string{schema1.MediaTypeManifest, schema2.MediaTypeManifest}
	digest, mediaType, payload, err := reg.PullManifest(tag, acceptMediaTypes)
	if err != nil {
		log.Errorf("an error occurred while pulling manifest of %s:%s from source registry: %v",
			reg.Name, tag, err)
		return "", nil, err
	}
	log.Infof("manifest of %s:%s pulled successfully from source registry: %s %s",
		reg.Name, tag, digest, reg.Endpoint)

	if strings.Contains(mediaType, "application/json") {
		mediaType = schema1.MediaTypeManifest
	}

	manifest, _, err := registry.UnMarshal(mediaType, payload)
	if err != nil {
		log.Errorf("an error occurred while parsing manifest: %v", err)
		return "", nil, err
	}

	return digest, manifest, nil
}

//CopyReposition ...
func (rep *Replication) CopyReposition(name string) error {
	source, err := rep.Source.Repository(name)
	target, err := rep.Target.Repository(name)
	sTags, err := source.ListTag()
	if err != nil {
		return err
	}
	//status := rep.GetStatus(name)
	for _, sTag := range sTags {
		/*status := rep.GetStatus(name, sTag)
		if status == "done" {
			continue
		}*/
		rep.SetStatus(name, sTag, "pending")
	}
	var lastError error
	//tTags, err := target.ListTag()
	for _, sTag := range sTags {
		status := rep.GetStatus(name, sTag)
		if status == "done" {
			continue
		}
		fmt.Printf("status %s %s %s\n", name, sTag, status)
		rep.SetStatus(name, sTag, "coping")
		var digest [2]string
		var manifest [2]distribution.Manifest
		var err [2]error
		var repo [2]*registry.Repository
		repo[0] = source
		repo[1] = target
		time.Sleep(time.Second)
		rep.SetStatus(name, sTag, "pullManifest")
		wg := sync.WaitGroup{}
		for i := 0; i < 2; i++ {
			//rep.PullManifest(source, st)
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				digest[i], manifest[i], err[i] = rep.pullManifest(repo[i], sTag)
			}(i)
		}
		wg.Wait()

		if err[0] != nil {
			lastError = err[0]
			rep.SetStatus(name, sTag, "error")
			continue
		}
		if digest[0] == digest[1] {
			log.Infof("manifest %s of %s:%s already exists on the destination registry", digest[0], target.Name, sTag)
			rep.SetStatus(name, sTag, "done")
			continue
		}
		rep.SetStatus(name, sTag, "copyLayers")
		if err := rep.transferLayers(source, target, sTag, manifest[0].References()); err != nil {
			lastError = err
			rep.SetStatus(name, sTag, "error")
			continue
		}
		rep.SetStatus(name, sTag, "pushManifest")
		if err := rep.pushManifest(target, sTag, digest[0], manifest[0]); err != nil {
			lastError = err
			rep.SetStatus(name, sTag, "error")
			continue
		}
		rep.SetStatus(name, sTag, "done")
	}
	return lastError
}

func (rep *Replication) pushManifest(target *registry.Repository, tag, digest string, manifest distribution.Manifest) error {

	repository := target.Name
	dgt, exist, err := target.ManifestExist(tag)
	if err != nil {
		log.Warningf("an error occurred while checking the existence of manifest of %s:%s on the destination registry: %v, try to push manifest",
			repository, tag, err)
	} else {
		if exist && dgt == digest {
			log.Infof("manifest of %s:%s exists on the destination registry, skip manifest pushing",
				repository, tag)
			return nil
		}
	}

	mediaType, data, err := manifest.Payload()
	if err != nil {
		log.Errorf("an error occurred while getting payload of manifest for %s:%s : %v",
			repository, tag, err)
		return err
	}

	if _, err = target.PushManifest(tag, mediaType, data); err != nil {
		log.Errorf("an error occurred while pushing manifest of %s:%s to the destination registry: %v",
			repository, tag, err)
		return err
	}
	log.Infof("manifest of %s:%s has been pushed to the destination registry",
		repository, tag)

	return nil
}

func (rep *Replication) transferLayers(src, target *registry.Repository, tag string, blobs []distribution.Descriptor) error {
	repository := src.Name
	// all blobs(layers and config)
	s := NewSemaphonre(10)
	errs := make([]error, len(blobs))
	for i, blob := range blobs {
		s.Add()
		go func(i int, blob distribution.Descriptor) {
			defer s.Done()
			digest := blob.Digest.String()
			exist, err := target.BlobExist(digest)
			if err != nil {
				log.Errorf("an error occurred while checking existence of blob %s of %s:%s on destination registry: %v",
					digest, repository, tag, err)
				errs[i] = err
			}
			if exist {
				log.Infof("blob %s of %s:%s already exists on the destination registry, skip",
					digest, repository, tag)
				return
			}

			log.Infof("transferring blob %s of %s:%s to the destination registry ...",
				digest, repository, tag)
			size, data, err := src.PullBlob(digest)
			if err != nil {
				log.Errorf("an error occurred while pulling blob %s of %s:%s from the source registry: %v",
					digest, repository, tag, err)
				errs[i] = err
				return
			}
			if data != nil {
				defer data.Close()
			}
			log.Infof("transferring push blob %s ", digest)

			if err = target.PushBlob(digest, size, data); err != nil {
				log.Errorf("an error occurred while pushing blob %s of %s:%s to the distination registry: %v",
					digest, repository, tag, err)
				errs[i] = err
				return
			}
			log.Infof("blob %s of %s:%s transferred to the destination registry completed",
				digest, repository, tag)
		}(i, blob)
	}
	s.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (rep *Replication) hasProject(r string) bool {
	if len(rep.Filter) == 0 {
		return true
	}
	for _, v := range rep.Filter {
		if strings.Contains(r, v) {
			return true
		}
	}
	return false
}

func (rep *Replication) replicate(reg *registry.Registry) error {
	for {
		repositories, err := reg.Catalog()
		if err != nil {
			return err
		}
		for _, r := range repositories {
			if !rep.hasProject(r) {
				continue
			}
			rep.SetStatus(r, "", "pending")
		}
		s := NewSemaphonre(3)
		for _, r := range repositories {
			if !rep.hasProject(r) {
				continue
			}
			s.Add()
			go func(r string) {
				defer s.Done()
				rep.SetStatus(r, "", "coping")
				if err := rep.CopyReposition(r); err != nil {
					rep.SetStatus(r, "", "error")
				} else {
					rep.SetStatus(r, "", "done")
				}
			}(r)
		}
		s.Wait()
		fmt.Printf("repos %v\n", repositories)
		if len(repositories) == 0 {
			time.Sleep(time.Second * time.Duration(delay))
			continue

		}

		repo, err := rep.Source.Repository(repositories[0])
		if err != nil {
			return err
		}
		tags, err := repo.ListTag()
		fmt.Printf("tag %v\n", tags)
		time.Sleep(time.Second * time.Duration(delay))
	}
}

//Run ...
func (rep *Replication) Run() error {
	reg, err := rep.Source.Registry()
	if err != nil {
		panic(err)
	}
	err = reg.Ping()
	if err != nil {
		panic(err)
	}
	rep.replicate(reg)
	return nil
}

//Config ..
type Config struct {
	Replications []*Replication
}

//Init ...
func (c *Config) Init(file string) {
	content, err := ioutil.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf(string(content))
	err = yaml.Unmarshal(content, c)
	if err != nil {
		panic(err)
	}
	for _, v := range c.Replications {
		v.Init()
	}

}

var configFile string
var delay int
var dev int

func init() {
	flag.StringVar(&configFile, "config", "config.yaml", "config file ")
	flag.IntVar(&delay, "delay", 10, "delay second to check all repository")
	flag.IntVar(&dev, "dev", 0, "dev mode")
}

var config Config

func toJSON(it interface{}) string {
	jt, _ := json.Marshal(it)
	return string(jt)
}

//GetStatus ...
func GetStatus(c *gin.Context) {
	rep := c.Param("rep")

	log.Printf("%v\n", toJSON(config.Replications))
	for _, v := range config.Replications {
		if v.Name == rep {
			fmt.Printf("start get status")
			v.mu.Lock()
			fmt.Printf("start status %#v", v.Projects)
			c.JSON(200, v.Projects)
			v.mu.Unlock()
			return
		}
	}
	c.JSON(404, map[string]string{})
}

func proxy(c *gin.Context) {
	rpURL, _ := url.Parse("http://127.0.0.1:8080")
	serv := httputil.NewSingleHostReverseProxy(rpURL)
	serv.ServeHTTP(c.Writer, c.Request)
}

//InitAPI init http api
func InitAPI(r *gin.Engine) {
	r.GET("/status/:rep", GetStatus)
	r.GET("/sync", func(c *gin.Context) {
		handler := jdata.Handler()
		handler.ServeHTTP(c.Writer, c.Request)
	})
	if dev == 1 {
		r.GET("/app.js", proxy)
		r.GET("/", proxy)
	}
}

func startHTTP() {
	eg := gin.Default()
	InitAPI(eg)
	err := eg.Run(":8081")
	if err != nil {
		panic(err)
	}
}

func main() {
	flag.Parse()
	jdata.Init(&mu)
	go startHTTP()
	config.Init(configFile)
	wait := sync.WaitGroup{}
	for _, v := range config.Replications {
		wait.Add(1)
		fmt.Printf("start replication %#v", v)
		go func(r *Replication) {
			defer wait.Done()
			r.Run()
		}(v)
	}
	wait.Wait()
}
