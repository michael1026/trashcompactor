package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/michael1026/sessionManager"
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

func (c *SafeResources) AddAndPrintIfUnique(key string, url string, contentType string, jsonResult map[string]string) {
	c.mu.Lock()
	if !c.resources[key] {
		fmt.Println(url)
		c.resources[key] = true
		jsonResult[url] = contentType
	}
	c.mu.Unlock()
}

func (c *SafeResources) Value(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resources[key]
}

func buildHttpClient(jar *cookiejar.Jar) (c *http.Client) {
	transport := &http.Transport{
		MaxIdleConns:      30,
		IdleConnTimeout:   3 * time.Second,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		DisableKeepAlives: true,
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: time.Second,
		}).DialContext,
	}

	re := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	client := &http.Client{
		Transport:     transport,
		CheckRedirect: re,
		Timeout:       time.Second * 3,
		Jar:           jar,
	}

	return client
}

func main() {
	resources := SafeResources{resources: make(map[string]bool)}
	cookieFile := flag.String("C", "", "File containing cookie")
	threads := flag.Int("t", 5, "Number of concurrent threads")
	outputJson := flag.String("json", "", "Output as json")
	jsonResult := make(map[string]string)

	flag.Parse()

	jar := sessionManager.ReadCookieJson(*cookieFile)
	urls := make(chan string)

	client := buildHttpClient(jar)

	wg := sync.WaitGroup{}
	s := bufio.NewScanner(os.Stdin)

	for i := 0; i < *threads; i++ {
		wg.Add(1)

		go printUniqueContentURLs(urls, client, &wg, &resources, jsonResult)
	}

	for s.Scan() {
		urls <- s.Text()
	}

	close(urls)
	wg.Wait()

	if *outputJson != "" {
		jsonFile, err := json.Marshal(jsonResult)

		if err != nil {
			fmt.Printf("Error marshalling JSON: %s\n", err)
			return
		}

		err = ioutil.WriteFile(*outputJson, jsonFile, 0644)

		if err != nil {
			fmt.Printf("Error writing JSON to file: %s\n", err)
		}
	}
}

func printUniqueContentURLs(urls chan string, client *http.Client, wg *sync.WaitGroup, resources *SafeResources, jsonResult map[string]string) {
	defer wg.Done()

	for rawUrl := range urls {
		req, err := http.NewRequest("GET", rawUrl, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Connection", "close")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			resource := ""
			if len(resp.Header.Get("content-type")) >= 9 && resp.Header.Get("content-type")[:9] == "text/html" {
				doc, err := goquery.NewDocumentFromReader(resp.Body)

				if err != nil {
					continue
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

				resources.AddAndPrintIfUnique(resource, rawUrl, "text/html", jsonResult)
			} else if len(resp.Header.Get("content-type")) >= 16 && resp.Header.Get("content-type")[:16] == "application/json" {
				var resultMap map[string]interface{}
				body, err := ioutil.ReadAll(resp.Body)

				if err != nil {
					continue
				}

				err = json.Unmarshal([]byte(body), &resultMap)

				if err != nil {
					continue
				}

				resource = mapKeysToString(resultMap)

				resources.AddAndPrintIfUnique(resource, rawUrl, "application/json", jsonResult)
			}
		}
	}
}

func mapKeysToString(jsonMap map[string]interface{}) string {
	finalString := ""
	for k := range jsonMap {
		finalString += k
	}
	return finalString
}

func writeOutputToJson(result *SafeResources) {

}
