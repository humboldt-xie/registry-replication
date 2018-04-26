package main

import (
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"strings"
	"sync"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/vmware/harbor/src/common/utils/registry"
)

//Status ...
type Status struct {
	Status string
}

//Replication ...
type Replication struct {
	Name       string
	Source     Registry
	Target     Registry
	Status     map[string]Status
	Projects   []string
	Repository []map[string]Status
	mu         sync.Mutex
}

//Init ...
func (rep *Replication) Init() error {
	rep.Target.Init()
	rep.Source.Init()
	return nil
}

//GetStatus ...
func (rep *Replication) GetStatus(r string) *Status {
	rep.mu.Lock()
	defer rep.mu.Unlock()
	return &rep.Status[r]
}

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
		panic(err)
	}
	//tTags, err := target.ListTag()
	for _, sTag := range sTags {
		var digest [2]string
		var manifest [2]distribution.Manifest
		var err [2]error
		var repo [2]*registry.Repository
		repo[0] = source
		repo[1] = target
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
			return err[0]
		}
		if err[1] != nil {
			return err[1]
		}
		if digest[0] == digest[1] {
			log.Infof("manifest %s of %s:%s already exists on the destination registry", digest[0], target.Name, sTag)
			continue
		}
		if err := rep.transferLayers(source, target, sTag, manifest[0].References()); err != nil {
			return err
		}
		if err := rep.pushManifest(target, sTag, digest[0], manifest[0]); err != nil {
			return err
		}
	}
	return nil
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
	for _, blob := range blobs {
		digest := blob.Digest.String()
		exist, err := target.BlobExist(digest)
		if err != nil {
			log.Errorf("an error occurred while checking existence of blob %s of %s:%s on destination registry: %v",
				digest, repository, tag, err)
			return err
		}
		if exist {
			log.Infof("blob %s of %s:%s already exists on the destination registry, skip",
				digest, repository, tag)
			continue
		}

		log.Infof("transferring blob %s of %s:%s to the destination registry ...",
			digest, repository, tag)
		size, data, err := src.PullBlob(digest)
		if err != nil {
			log.Errorf("an error occurred while pulling blob %s of %s:%s from the source registry: %v",
				digest, repository, tag, err)
			return err
		}
		if data != nil {
			defer data.Close()
		}
		log.Infof("transferring push blob %s ", digest)

		if err = target.PushBlob(digest, size, data); err != nil {
			log.Errorf("an error occurred while pushing blob %s of %s:%s to the distination registry: %v",
				digest, repository, tag, err)
			return err
		}
		log.Infof("blob %s of %s:%s transferred to the destination registry completed",
			digest, repository, tag)
	}

	return nil
}

func (rep *Replication) hasProject(r string) bool {
	for _, v := range rep.Projects {
		if strings.Contains(r, v) {
			return true
		}
	}
	return false
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
	repositories, err := reg.Catalog()
	if err != nil {
		panic(err)
	}
	for _, r := range repositories {
		rep.GetStatus.Status = "pending"
	}
	for _, r := range repositories {
		if rep.hasProject(r) {
			rep.CopyReposition(r)
		}
	}
	fmt.Printf("repos %v\n", repositories)

	repo, err := rep.Source.Repository(repositories[0])
	if err != nil {
		panic(err)
	}
	tags, err := repo.ListTag()
	fmt.Printf("tag %v\n", tags)
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

func init() {
	flag.StringVar(&configFile, "config", "config.yaml", "config file ")
}

var config Config

func main() {
	flag.Parse()
	config.Init(configFile)
	wait := sync.WaitGroup{}
	for _, v := range config.Replications {
		wait.Add(1)
		go func(r *Replication) {
			defer wait.Done()
			r.Run()
		}(v)
	}
	wait.Wait()
}
