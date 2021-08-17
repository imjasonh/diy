package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"sort"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/imdario/mergo"
	yaml "gopkg.in/yaml.v3"
)

var (
	fn  = flag.String("f", "", "config file")
	dst = flag.String("t", "", "tag to push")
)

func main() {
	flag.Parse()

	b, err := ioutil.ReadFile(*fn)
	if err != nil {
		log.Fatal(err)
	}

	var cfg config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		log.Fatal(err)
	}

	var img v1.Image = empty.Image
	if cfg.Base != "" {
		br, err := name.ParseReference(cfg.Base)
		if err != nil {
			log.Fatal(err)
		}
		img, err = remote.Image(br)
		if err != nil {
			log.Fatal(err)
		}
	}

	for _, l := range cfg.Layers {
		// sort for reproducibility.
		sort.Slice(l.Files, func(i, j int) bool { return l.Files[i].Name < l.Files[j].Name })
		filenames := map[string]struct{}{}
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		for _, ff := range l.Files {
			fn := filepath.Clean(ff.Name)
			if fn == "" {
				log.Fatal("filename is required")
			}
			if _, found := filenames[fn]; found {
				log.Fatalf("duplicate file path: %s", fn)
			}
			filenames[fn] = struct{}{}

			if ff.Contents != "" && ff.Data != "" {
				log.Fatal("cannot specify file contents and data")
			}
			size := len(ff.Contents)
			data := []byte(ff.Contents)
			if ff.Data != "" {
				data, err = base64.StdEncoding.DecodeString(ff.Data)
				if err != nil {
					log.Fatal(err)
				}
				size = len(data)
			}

			if err := tw.WriteHeader(&tar.Header{
				Name: filepath.Clean(ff.Name),
				Size: int64(size),
				Mode: ff.Mode,
			}); err != nil {
				log.Fatal(err)
			}
			if _, err := tw.Write(data); err != nil {
				log.Fatal(err)
			}
		}
		if err := tw.Flush(); err != nil {
			log.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			log.Fatal(err)
		}

		layer, err := tarball.LayerFromReader(&buf, tarball.WithCompressionLevel(gzip.BestCompression))
		if err != nil {
			log.Fatal(err)
		}
		img, err = mutate.AppendLayers(img, layer)
	}

	// Apply annotations.
	img = mutate.Annotations(img, cfg.Annotations).(v1.Image)

	// Merge YAML config into base image config.
	if cfg.Config != nil {
		icfg, err := img.ConfigFile()
		if err != nil {
			log.Fatal(err)
		}
		if err := mergo.Merge(&icfg.Config, cfg.Config, mergo.WithOverride); err != nil {
			log.Fatal(err)
		}
		img, err = mutate.ConfigFile(img, icfg)
		if err != nil {
			log.Fatal(err)
		}
	}

	dstref, err := name.ParseReference(*dst)
	if err != nil {
		log.Fatal(err)
	}
	if err := remote.Write(dstref, img, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
		log.Fatal(err)
	}
	d, err := img.Digest()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(dstref.Context().Digest(d.String()))
}

type config struct {
	Base        string
	Annotations map[string]string
	Layers      []layer
	Config      *v1.Config
}

type layer struct {
	Files []file
}

type file struct {
	Name     string
	Contents string
	Mode     int64
	Data     string
}
