package main

type Instance struct {
	id            int
	containerName string
	description   string
	dirName       string
	keepInventory bool
	prefix        string
	s3Bucket      string
	active        bool
	workingPath   string
}
