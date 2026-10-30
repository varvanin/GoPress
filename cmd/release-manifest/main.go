// release-manifest is an offline release signing tool. Never ship its private key.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"gopress/internal/store"
	"gopress/internal/update"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func keyFromEnvironment() (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(os.Getenv("GOPRESS_RELEASE_PRIVATE_KEY"))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, errors.New("set GOPRESS_RELEASE_PRIVATE_KEY to a base64 Ed25519 private key")
	}
	derived := ed25519.NewKeyFromSeed(raw[:ed25519.SeedSize])
	if !ed25519.PrivateKey(raw).Equal(derived) {
		return nil, errors.New("invalid signing key")
	}
	return derived, nil
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("release-manifest keygen|public-key|sign")
	}
	if args[0] == "schema" {
		if len(args) != 1 {
			return errors.New("unexpected arguments")
		}
		fmt.Println(store.SchemaVersion)
		return nil
	}
	if args[0] == "validate-version" {
		if len(args) != 2 {
			return errors.New("version required")
		}
		_, err := update.Compare(args[1], args[1])
		return err
	}
	if args[0] == "keygen" {
		if len(args) != 2 {
			return errors.New("keygen requires a new private-key file path")
		}
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(args[1], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = file.WriteString(base64.StdEncoding.EncodeToString(private) + "\n")
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Println(base64.StdEncoding.EncodeToString(public))
		return nil
	}
	key, err := keyFromEnvironment()
	if err != nil {
		return err
	}
	if args[0] == "public-key" {
		if len(args) != 1 {
			return errors.New("unexpected arguments")
		}
		fmt.Println(base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)))
		return nil
	}
	if args[0] != "sign" {
		return errors.New("unknown command")
	}
	flags := flag.NewFlagSet("sign", flag.ContinueOnError)
	version := flags.String("version", "", "stable release version without v")
	minimum := flags.String("minimum-version", "", "oldest supported source version")
	schema := flags.Int("schema", store.SchemaVersion, "expected SQLite schema after startup")
	image := flags.String("image", "", "immutable ghcr.io image digest")
	notes := flags.String("notes-file", "", "UTF-8 release notes")
	output := flags.String("output", "update.json", "manifest path")
	if err = flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	raw, err := os.ReadFile(*notes)
	if err != nil {
		return err
	}
	release := update.Release{Format: 1, Version: *version, MinimumVersion: *minimum, Schema: *schema, Image: *image, Notes: string(raw)}
	if *schema != store.SchemaVersion {
		return errors.New("schema must match the release build")
	}
	repo := strings.Split(*image, "@sha256:")[0]
	if !strings.HasPrefix(repo, "ghcr.io/") || release.Validate(repo) != nil {
		return errors.New("invalid release metadata")
	}
	manifest, err := json.MarshalIndent(release, "", "  ")
	if err != nil {
		return err
	}
	manifest = append(manifest, '\n')
	if len(manifest) > 20000 {
		return errors.New("release manifest is too large")
	}
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(key, manifest)) + "\n"
	if err = os.WriteFile(*output, manifest, 0644); err != nil {
		return err
	}
	return os.WriteFile(*output+".sig", []byte(signature), 0644)
}
