package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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
	if err := yaml.Unmarshal(b, &cfg); err != nil {
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

		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		for _, ff := range l.Files {
			if err := tw.WriteHeader(&tar.Header{
				Name: filepath.Clean(ff.Name),
				Size: int64(len(ff.Contents)),
			}); err != nil {
				log.Fatal(err)
			}
			if _, err := tw.Write([]byte(ff.Contents)); err != nil {
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
}

type layer struct {
	Files []file
}

type file struct {
	Name     string
	Contents string
	// TODO: chmod, bytes
}
