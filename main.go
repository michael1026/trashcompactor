package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/michael1026/sessionManager"
	"github.com/projectdiscovery/fastdialer/fastdialer"
)

type CookieInfo map[string]string

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

func buildHttpClient(jar cookiejar.Jar) (c *http.Client) {
	fastdialerOpts := fastdialer.DefaultOptions
	fastdialerOpts.EnableFallback = true
	dialer, err := fastdialer.NewDialer(fastdialerOpts)
	if err != nil {
		return
	}

	transport := &http.Transport{
		MaxIdleConns:      -1,
		IdleConnTimeout:   time.Second,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives: true,
		DialContext:       dialer.Dial,
	}

	re := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	client := &http.Client{
		Transport:     transport,
		CheckRedirect: re,
		Timeout:       time.Second * 3,
		Jar:           &jar,
	}

	return client
}

func main() {
	resources := SafeResources{resources: make(map[string]bool)}
	cookieFile := flag.String("C", "", "File containing cookie")

	flag.Parse()

	jar := sessionManager.ReadCookieJson(*cookieFile)

	client := buildHttpClient(*jar)

	wg := &sync.WaitGroup{}
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		wg.Add(1)
		time.Sleep(100 * time.Millisecond)
		go printUniqueContentURLs(s.Text(), client, wg, &resources)
	}

	wg.Wait()
}

func printUniqueContentURLs(rawUrl string, client *http.Client, wg *sync.WaitGroup, resources *SafeResources) {
	defer wg.Done()
	req, err := http.NewRequest("GET", rawUrl, nil)
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
			srcurl, err := url.Parse(src)

			if err != nil {
				return
			}

			srcurl.RawQuery = ""

			resource += srcurl.String()
		})

		if resources.Value(resource) {
			return
		}

		resources.AddResource(resource)
		fmt.Println(rawUrl)
	}
}
