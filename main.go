package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/imjasonh/diy/pkg"
	yaml "gopkg.in/yaml.v3"
)

var (
	fn      = flag.String("f", "", "config file")
	dst     = flag.String("t", "", "tag to push")
	verbose = flag.Bool("v", false, "verbose logging")
)

func main() {
	flag.Parse()

	b, err := ioutil.ReadFile(*fn)
	if err != nil {
		log.Fatal(err)
	}

	var cfg pkg.Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		log.Fatal(err)
	}

	if err := pkg.Resolve(&cfg, *verbose); err != nil {
		log.Fatal(err)
	}
	img, err := pkg.Build(cfg, *verbose)
	if err != nil {
		log.Fatal(err)
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
