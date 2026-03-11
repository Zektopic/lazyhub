package lib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"text/template"
)

type Client struct {
	OfficialURL *url.URL
	HTTPClient  *http.Client
}

type Item struct {
	ID              int      `json:"id"`
	Name            string   `json:"name,repo"`
	FullName        string   `json:"full_name"`
	URL             string   `json:"repo_link"`
	HTMLURL         string   `json:"html_url"`
	CloneURL        string   `json:"clone_url"`
	Description     string   `json:"description"`
	Desc            string   `json:"desc"`
	StargazersCount int      `json:"stargazers_count,stars"`
	Stars           string   `json:"stars"`
	Watchers        int      `json:"watchers"`
	Topics          []string `json:"topics"`
	Language        string   `json:"language"`
	Lang            string   `json:"lang"`
	DefaultBranch   string   `json:"default_branch"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	DataSource      string
}

type Readme struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	HTMLURL     string `json:"html_url"`
	DownloadURL string `json:"download_url"`
	Content     string `json:"content"`
}

type Result struct {
	Items []Item `json:"items"`
}

func (item *Item) GetRepositoryName() string {
	name := item.FullName
	if name == "" {
		if item.URL == "" {
			return item.Name
		}
		u, err := url.Parse(item.URL)
		if err != nil {
			return item.Name
		}
		name = u.Path[1:]
	}
	return name
}

func (item *Item) GetStars() int {
	stars, _ := strconv.Atoi(strings.Replace(item.Stars, ",", "", -1))
	if stars == 0 {
		return item.StargazersCount
	}
	return stars
}

func (item *Item) GetRepositoryURL() string {
	u := item.HTMLURL
	if u == "" {
		return item.URL
	}
	return u
}
func (item *Item) GetDescription() string {
	description := item.Description
	if description == "" {
		return item.Desc
	}
	return description
}
func (item *Item) GetLanguage() string {
	language := item.Language
	if language == "" {
		return item.Lang
	}
	return language
}
func (item *Item) GetCloneURL() string {
	u := item.GetRepositoryURL()
	if u == "" {
		return ""
	}
	if !strings.HasSuffix(u, ".git") {
		return u + ".git"
	}
	return u
}

func (item *Item) String() string {
	const officialTemplateText = `
	Name       : {{.GetRepositoryName}}
	URL        : {{.GetRepositoryURL}}
	Star       : ⭐️ {{.StargazersCount}}
	Clone URL  : {{.GetCloneURL}}
	Description: {{.Description}}
	Watchers   : {{.Watchers}}
	Topics     : {{.Topics}}
	Language   : {{.Language}}
	CreatedAt  : {{.CreatedAt}}
	UpdatedAt  : {{.UpdatedAt}}
	`
	const trendingTemplateText = `
	Name       : {{.GetRepositoryName}}
	URL        : {{.GetRepositoryURL}}
	Star       : ⭐️ {{.Stars}}
	Clone URL  : {{.GetCloneURL}}
	Description: {{.GetDescription}}
	Language   : {{.GetLanguage}}
	`
	templateText := trendingTemplateText
	if item.DataSource == "OfficialAPI" {
		templateText = officialTemplateText
	}
	tmpl, err := template.New("Repository").Parse(templateText)
	if err != nil {
		return fmt.Sprintf("Name: %s", item.GetRepositoryName())
	}
	var doc bytes.Buffer
	if err := tmpl.Execute(&doc, item); err != nil {
		return fmt.Sprintf("Name: %s", item.GetRepositoryName())
	}
	return doc.String()
}

func (result *Result) Draw(writer io.Writer) error {
	if result == nil || len(result.Items) == 0 {
		fmt.Fprintln(writer, "  No repositories found.")
		return nil
	}
	for _, item := range result.Items {
		starText := " ⭐️ " + strconv.Itoa(item.GetStars())
		fmt.Fprintf(writer, "%-10.10s\033[32m%s\033[0m\n", starText, item.GetRepositoryName())
	}
	return nil
}

func NewClient() (*Client, error) {
	officialURL, err := url.Parse("https://api.github.com")
	if err != nil {
		return nil, err
	}
	return &Client{
		OfficialURL: officialURL,
		HTTPClient:  http.DefaultClient,
	}, nil
}

func (client *Client) SearchRepository(query string) (*Result, error) {
	u := *client.OfficialURL
	u.Path = path.Join(u.Path, "search", "repositories")
	req, err := http.NewRequest("GET", u.String()+"?q="+query, nil)
	if err != nil {
		return &Result{Items: []Item{}}, err
	}
	req.Header.Add("Accept", "application/vnd.github.mercy-preview+json")
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return &Result{Items: []Item{}}, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return &Result{Items: []Item{}}, err
	}
	var result *Result
	if err = json.Unmarshal(body, &result); err != nil {
		return &Result{Items: []Item{}}, err
	}
	if result == nil {
		return &Result{Items: []Item{}}, nil
	}
	for i := range result.Items {
		result.Items[i].DataSource = "OfficialAPI"
	}
	return result, nil
}

func (client *Client) GetReadme(item Item) (*Readme, error) {
	u := *client.OfficialURL
	u.Path = path.Join(u.Path, "repos", item.GetRepositoryName(), "readme")
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/vnd.github.mercy-preview+json")
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var readme *Readme
	if err = json.Unmarshal(body, &readme); err != nil {
		return nil, err
	}
	return readme, nil
}

// GetTrendingRepository scrapes GitHub's trending page directly
// since all third-party trending APIs (trendings.herokuapp.com, ghapi.huchen.dev) are dead.
func (client *Client) GetTrendingRepository(language string, since string) (*Result, error) {
	// Build the GitHub trending URL
	trendingURL := "https://github.com/trending"
	if language != "" {
		trendingURL += "/" + language
	}
	params := url.Values{}
	if since != "" {
		params.Set("since", since)
	}
	if len(params) > 0 {
		trendingURL += "?" + params.Encode()
	}

	req, err := http.NewRequest("GET", trendingURL, nil)
	if err != nil {
		return &Result{Items: []Item{}}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) lazyhub/1.0")
	req.Header.Set("Accept", "text/html")

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return &Result{Items: []Item{}}, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return &Result{Items: []Item{}}, err
	}

	html := string(body)
	items := parseTrendingHTML(html)

	if len(items) == 0 {
		return &Result{Items: []Item{}}, nil
	}

	return &Result{Items: items}, nil
}

// parseTrendingHTML extracts repository data from the GitHub trending page HTML.
// It uses regex to parse the article.Box-row elements.
func parseTrendingHTML(html string) []Item {
	var items []Item

	// Match each repo article block
	reArticle := regexp.MustCompile(`(?s)<article class="Box-row">(.+?)</article>`)
	articles := reArticle.FindAllStringSubmatch(html, -1)

	// Repo name pattern: <h2 ...> <a href="/owner/repo" ...>
	reRepo := regexp.MustCompile(`<h2[^>]*>[\s\S]*?<a[^>]*href="(/[^"]+)"`)
	// Description
	reDesc := regexp.MustCompile(`<p class="col-9[^"]*"[^>]*>([\s\S]*?)</p>`)
	// Language
	reLang := regexp.MustCompile(`<span itemprop="programmingLanguage">(.*?)</span>`)
	// Stars count — the number appears on a separate line inside the stargazers <a> tag, after an SVG
	reStars := regexp.MustCompile(`(?s)<a[^>]*href="/[^/]+/[^/]+/stargazers"[^>]*>.*?>\s*([\d,]+)\s*</a>`)
	// Today's stars
	reTodayStars := regexp.MustCompile(`([\d,]+)\s*stars\s*(today|this week|this month)`)

	for _, match := range articles {
		block := match[1]

		// Extract repo path
		repoMatch := reRepo.FindStringSubmatch(block)
		if repoMatch == nil {
			continue
		}
		repoPath := strings.TrimSpace(repoMatch[1])
		repoPath = strings.TrimPrefix(repoPath, "/")

		// Extract description
		desc := ""
		descMatch := reDesc.FindStringSubmatch(block)
		if descMatch != nil {
			desc = strings.TrimSpace(descMatch[1])
			// Strip HTML tags from description
			reTag := regexp.MustCompile(`<[^>]*>`)
			desc = reTag.ReplaceAllString(desc, "")
			desc = strings.TrimSpace(desc)
		}

		// Extract language
		lang := ""
		langMatch := reLang.FindStringSubmatch(block)
		if langMatch != nil {
			lang = strings.TrimSpace(langMatch[1])
		}

		// Extract stars
		stars := "0"
		starsMatch := reStars.FindStringSubmatch(block)
		if starsMatch != nil {
			stars = strings.TrimSpace(starsMatch[1])
		}

		// Extract today's stars (for display purposes)
		todayStars := ""
		todayMatch := reTodayStars.FindStringSubmatch(block)
		if todayMatch != nil {
			todayStars = todayMatch[1] + " stars " + todayMatch[2]
		}
		_ = todayStars

		parts := strings.SplitN(repoPath, "/", 2)
		name := repoPath
		if len(parts) == 2 {
			name = parts[1]
		}

		item := Item{
			Name:       name,
			FullName:   repoPath,
			URL:        "https://github.com/" + repoPath,
			Stars:      stars,
			Language:   lang,
			Lang:       lang,
			Desc:       desc,
			DataSource: "TrendingAPI",
		}
		items = append(items, item)
	}

	return items
}
