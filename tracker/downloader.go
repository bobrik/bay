package main

import "sync"
import "os"
import "path"
import "net/http"
import "io"
import "crypto/sha1"
import "fmt"
import "log"

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

func (d *Downloader) Download(url string) (file string, err error) {
	name := fmt.Sprintf("%x", sha1.Sum([]byte(url)))
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
		return d.Download(url)
	}

	log.Println("starting new download")

	// new mutex: create it and put into map
	m := &sync.Mutex{}
	d.current[name] = m

	m.Lock()
	defer m.Unlock()

	d.mutex.Unlock()

	err = d.save(url, file)
	if err != nil {
		return
	}

	// delete mutex from the map
	d.mutex.Lock()
	delete(d.current, name)
	d.mutex.Unlock()

	return
}

func (d *Downloader) save(url, file string) error {
	out, err := os.Create(file + ".downloading")
	if err != nil {
		return err
	}

	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return os.Rename(file+".downloading", file)
}
