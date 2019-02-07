package main

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"sync"
)

type Downloader struct {
	root    string
	mutex   sync.Mutex
	current map[string]*sync.Mutex
}

func NewDownloader(root string) *Downloader {
	return &Downloader{
		root:    root,
		mutex:   sync.Mutex{},
		current: map[string]*sync.Mutex{},
	}
}

func (d *Downloader) Download(req *http.Request) (file string, err error) {
	name := fmt.Sprintf("%x", sha1.Sum([]byte(req.URL.String())))
	file = path.Join(d.root, name)
	if _, err = os.Stat(file); err == nil {
		return
	}

	d.mutex.Lock()

	// existing mutex: just wait for it to unlock
	if e, ok := d.current[name]; ok {
		log.Println("waiting for current download to finish")
		d.mutex.Unlock()
		e.Lock()

		// here file is downloaded already or failed
		// and "e" is no longer referenced from the map
		return d.Download(req)
	}

	log.Println("starting new download")

	// new mutex: create it and put into map
	m := &sync.Mutex{}
	d.current[name] = m

	m.Lock()
	defer m.Unlock()

	d.mutex.Unlock()

	err = d.save(req, file)
	if err != nil {
		return
	}

	// delete mutex from the map
	d.mutex.Lock()
	delete(d.current, name)
	d.mutex.Unlock()

	return
}

func (d *Downloader) save(req *http.Request, file string) error {
	out, err := os.Create(file + ".downloading")
	if err != nil {
		return err
	}

	defer out.Close()

	downloadClient := http.DefaultClient
	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	log.Printf("Status: %s\n", resp.Status)
	if resp.StatusCode != 200 {
		os.Remove(file + ".downloading")
		return errors.New("Failed to download: " + resp.Status)
	}

	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return os.Rename(file+".downloading", file)
}
