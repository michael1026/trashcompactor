package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type SafeResources struct {
	mu        sync.Mutex
	resources map[string]bool
}

func (c *SafeResources) AddResource(key string) {
	c.mu.Lock()
	c.resources[key] = true
	c.mu.Unlock()
}

func (c *SafeResources) Value(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resources[key]
}

func main() {
	resources := SafeResources{resources: make(map[string]bool)}
	flag.Parse()
	transport := &http.Transport{
		MaxIdleConns:    30,
		IdleConnTimeout: time.Second,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout:   time.Second * 10,
			KeepAlive: time.Second,
		}).DialContext,
	}

	re := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	client := &http.Client{
		Transport:     transport,
		CheckRedirect: re,
		Timeout:       time.Second * 10,
	}

	wg := &sync.WaitGroup{}
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		wg.Add(1)
		go printUniqueContentURLs(s.Text(), client, wg, &resources)
	}

	wg.Wait()
}

func printUniqueContentURLs(url string, client *http.Client, wg *sync.WaitGroup, resources *SafeResources) {
	defer wg.Done()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Connection", "close")

	resp, err := client.Do(req)
	if err != nil {
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK && len(resp.Header.Get("content-type")) >= 9 && resp.Header.Get("content-type")[:9] == "text/html" {
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		resource := ""

		if err != nil {
			return
		}

		doc.Find("script[src]").Each(func(index int, item *goquery.Selection) {
			src, _ := item.Attr("src")
			resource += src
		})

		if resources.Value(resource) {
			return
		}

		resources.AddResource(resource)
		fmt.Printf("%s\n", url)
	}
}
