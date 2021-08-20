package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/imjasonh/diy/pkg"
	"github.com/spf13/cobra"
	yaml "gopkg.in/yaml.v3"
)

func main() {
	root := &cobra.Command{
		Use:   "diy",
		Short: "DIY is a tool to declaratively build container images.",
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Usage() },
	}
	root.AddCommand(
		build(),
		resolve(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func build() *cobra.Command {
	var fn string
	var verbose bool
	build := &cobra.Command{
		Use:          "build",
		Short:        "Build and push an image",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := os.Getenv("DIY_REPO")
			if repo == "" {
				return errors.New("must set DIY_REPO env var")
			}

			// Parse the config YAML.
			b, err := ioutil.ReadFile(fn)
			if err != nil {
				return err
			}
			var cfg pkg.Config
			dec := yaml.NewDecoder(bytes.NewReader(b))
			dec.KnownFields(true)
			if err := dec.Decode(&cfg); err != nil {
				return err
			}

			// Resolve and build the image.
			if err := pkg.Resolve(&cfg, verbose); err != nil {
				return err
			}
			img, err := pkg.Build(cfg, verbose)
			if err != nil {
				return err
			}

			// Push the image.
			dstref, err := name.ParseReference(path.Join(repo, cfg.Name))
			if err != nil {
				return err
			}
			if err := remote.Write(dstref, img, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
				return err
			}
			d, err := img.Digest()
			if err != nil {
				return err
			}
			fmt.Println(dstref.Context().Digest(d.String()))
			return nil
		},
	}
	build.Flags().StringVarP(&fn, "filename", "f", "config.yaml", "Config file describing the image")
	build.Flags().BoolVarP(&verbose, "verbose", "v", false, "If true, verbosely log")
	return build
}

func resolve() *cobra.Command {
	var fn string
	var verbose bool
	resolve := &cobra.Command{
		Use:          "resolve",
		Short:        "Resolve mutable references in a config file",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse the config YAML.
			b, err := ioutil.ReadFile(fn)
			if err != nil {
				return err
			}
			var cfg pkg.Config
			dec := yaml.NewDecoder(bytes.NewReader(b))
			dec.KnownFields(true)
			if err := dec.Decode(&cfg); err != nil {
				return err
			}

			// Resolve and build the image.
			if err := pkg.Resolve(&cfg, verbose); err != nil {
				return err
			}

			enc := yaml.NewEncoder(os.Stdout)
			enc.SetIndent(2) // God's one true YAML indentation level.
			if err := enc.Encode(cfg); err != nil {
				return err
			}
			return enc.Close()
		},
	}
	resolve.Flags().StringVarP(&fn, "filename", "f", "config.yaml", "Config file describing the image")
	return resolve
}
