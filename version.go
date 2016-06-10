package main

import "fmt"

type version struct {
	Major, Minor, Patch int
	Label               string
	Nick                string
}

var ver = version{
	Major: 0,
	Minor: 0,
	Patch: 2,
	Label: "alpha_1",
	Nick:  "Jack Bauer"}

// CommitHash may be set on the build command line:
// go build -ldflags "-X main.CommitHash=`git rev-parse HEAD`"
// var CommitHash string

const appName string = "dcrspy"

func (v version) String() string {
	if v.Label != "" {
		return fmt.Sprintf("%d.%d.%d-%s \"%s\"",
			v.Major, v.Minor, v.Patch, v.Label, v.Nick)
	}
	return fmt.Sprintf("%d.%d.%d \"%s\"",
		v.Major, v.Minor, v.Patch, v.Nick)
}
