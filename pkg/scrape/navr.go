package scrape

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
	"github.com/mozillazg/go-slugify"
	"github.com/thoas/go-funk"
	"github.com/xbapps/xbvr/pkg/models"
)

var naHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

const naUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

type naSceneResponse struct {
	Access         int    `json:"access"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	ThumbnailImage string `json:"thumbnailImage"`
	Duration       int    `json:"duration"`
	Tags           []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

func fetchNaughtyAmericaScene(sceneID string) (naSceneResponse, bool) {
	var result naSceneResponse

	req, err := http.NewRequest("POST", "https://api.naughtyapi.com/heresphere/"+sceneID, nil)
	if err != nil {
		return result, false
	}
	req.Header.Set("User-Agent", naUserAgent)

	resp, err := naHTTPClient.Do(req)
	if err != nil {
		return result, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return result, false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, false
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return result, false
	}

	if result.Access != 0 || result.Title == "" {
		return result, false
	}

	return result, true
}

type naScenesResponse struct {
	CurrentPage int          `json:"current_page"`
	LastPage    int          `json:"last_page"`
	Total       int          `json:"total"`
	Data        []naSceneAPI `json:"data"`
}

type naSceneAPI struct {
	ID            int                 `json:"id"`
	Title         string              `json:"title"`
	Length        int                 `json:"length"`
	PublishedDate string              `json:"published_date"`
	SceneURL      string              `json:"scene_url"`
	SiteName      string              `json:"site_name"`
	Synopsis      string              `json:"synopsis"`
	Tags          []string            `json:"tags"`
	Performers    map[string][]string `json:"performers"`
}

func hasVRTag(tags []string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, "Virtual Reality") || strings.EqualFold(t, "VR Porn") {
			return true
		}
	}
	return false
}

func fetchNAVRPage(page int) (naScenesResponse, error) {
	var r naScenesResponse
	u := fmt.Sprintf("https://api.naughtyapi.com/tools/scenes/scenes?page=%d", page)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return r, err
	}
	req.Header.Set("User-Agent", naUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://www.naughtyamerica.com/")

	resp, err := naHTTPClient.Do(req)
	if err != nil {
		return r, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return r, err
	}
	if resp.StatusCode != http.StatusOK {
		return r, fmt.Errorf("API returned %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return r, err
	}
	return r, nil
}

// fetchNAVRPages walks every page of the NaughtyAmerica scenes API and returns
// all scenes tagged as VR. Individual page failures are retried a few times
// and, if still failing, skipped (rather than aborting the whole pagination
// and discarding everything already collected).
func fetchNAVRPages(limitScraping bool) ([]naSceneAPI, error) {
	var all []naSceneAPI
	page := 1
	lastPage := 0
	const maxRetries = 3
	consecutiveFailures := 0
	const maxConsecutiveFailures = 5

	for {
		if limitScraping && page > 1 {
			break
		}

		var r naScenesResponse
		var err error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			log.Infof("NAVR: fetching API page %d (attempt %d/%d)", page, attempt, maxRetries)
			r, err = fetchNAVRPage(page)
			if err == nil {
				break
			}
			log.Warnf("NAVR: error fetching API page %d: %v", page, err)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		if err != nil {
			consecutiveFailures++
			log.Errorf("NAVR: giving up on API page %d after %d attempts: %v", page, maxRetries, err)
			if consecutiveFailures >= maxConsecutiveFailures {
				log.Errorf("NAVR: too many consecutive failed pages, stopping pagination at page %d", page)
				break
			}
			page++
			if lastPage > 0 && page > lastPage {
				break
			}
			continue
		}
		consecutiveFailures = 0
		lastPage = r.LastPage

		log.Infof("NAVR: API page %d/%d returned %d scenes", page, r.LastPage, len(r.Data))

		for _, s := range r.Data {
			if !hasVRTag(s.Tags) {
				continue
			}
			if !strings.Contains(s.SceneURL, "www.naughtyamerica.com") {
				continue
			}
			all = append(all, s)
		}

		if r.LastPage == 0 || page >= r.LastPage {
			break
		}
		page++
		time.Sleep(200 * time.Millisecond)
	}
	log.Infof("NAVR: collected %d VR scenes from API", len(all))
	return all, nil
}

func parseNADate(s string) string {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"Jan 2, 2006", "January 2, 2006", "01/02/2006", "01-02-2006", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

func getNaughtyAmericaSceneID(sceneURL string) string {
	tmp := strings.Split(sceneURL, "-")
	return tmp[len(tmp)-1]
}

func parseNADuration(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	total := 0

	if h := regexp.MustCompile(`(\d+)\s*h`).FindStringSubmatch(s); h != nil {
		if n, err := strconv.Atoi(h[1]); err == nil {
			total += n * 60
		}
	}
	if m := regexp.MustCompile(`(\d+)\s*min`).FindStringSubmatch(s); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			total += n
		}
	}

	return total
}

func getNABasePrefixSlug(imageURL string) (string, string) {
	parts := strings.Split(imageURL, "/scenes/")
	if len(parts) < 2 {
		return "", ""
	}
	segs := strings.Split(parts[1], "/")
	if len(segs) < 2 {
		return "", ""
	}
	return segs[0], segs[1]
}

func buildNACovers(imageURL string) ([]string, []string) {
	var covers, gallery []string
	if imageURL == "" {
		return covers, gallery
	}

	imageURL = strings.Split(imageURL, "?")[0]
	m := regexp.MustCompile(`^(.*scene/)`).FindStringSubmatch(imageURL)
	if len(m) < 2 {
		return covers, gallery
	}
	base := m[1]

	covers = append(covers, imageURL)
	covers = append(covers, base+"vertical/1182x1788c.jpg")
	covers = append(covers, base+"horizontal/1182x777c.jpg")

	gallery = append(gallery, base+"image1/1182x777c.jpg")
	gallery = append(gallery, base+"image2/1000x563c.jpg")
	gallery = append(gallery, base+"image3/1000x563c.jpg")
	gallery = append(gallery, base+"image4/1000x563c.jpg")
	gallery = append(gallery, base+"image3/1279x852c.jpg")

	return covers, gallery
}

func NaughtyAmericaVR(wg *models.ScrapeWG, updateSite bool, knownScenes []string, out chan<- models.ScrapedScene, singleSceneURL string, singeScrapeAdditionalInfo string, limitScraping bool) error {
	defer wg.Done()
	scraperID := "naughtyamericavr"
	siteID := "NaughtyAmerica VR"
	logScrapeStart(scraperID, siteID)

	sceneCollector := createCollector("www.naughtyamerica.com")

	// Scene detail page
	sceneCollector.OnHTML(`html`, func(e *colly.HTMLElement) {
		sc := models.ScrapedScene{}
		sc.ScraperID = scraperID
		sc.SceneType = "VR"
		sc.Studio = "NaughtyAmerica"
		sc.Site = siteID
		sc.HomepageURL = strings.Split(e.Request.URL.String(), "?")[0]
		sc.MembersUrl = strings.Replace(sc.HomepageURL, "https://www.naughtyamerica.com/", "https://members.naughtyamerica.com/", 1)
		sc.SiteID = getNaughtyAmericaSceneID(sc.HomepageURL)
		sc.SceneID = slugify.Slugify(sc.Site) + "-" + sc.SiteID

		// Load the API scene data passed from the listing
		var apiScene naSceneAPI
		if apiJSON := e.Request.Ctx.Get("apiScene"); apiJSON != "" {
			_ = json.Unmarshal([]byte(apiJSON), &apiScene)
		}

		// Title: site title + scene title (scoped to .scene-info to avoid matching related-scenes cards)
		info := e.DOM.Find(`.scene-info`).First()

		siteTitle := strings.TrimSpace(info.Find(`a.site-title`).First().Text())
		if siteTitle == "" {
			siteTitle = apiScene.SiteName
		}

		sceneTitle := strings.TrimSpace(info.Find(`h1.scene-title`).First().Text())
		if sceneTitle == "" {
			sceneTitle = apiScene.Title
		}

		if siteTitle != "" && sceneTitle != "" {
			sc.Title = siteTitle + " " + sceneTitle
		} else if sceneTitle != "" {
			sc.Title = sceneTitle
		} else if siteTitle != "" {
			sc.Title = siteTitle
		}

		// Date
		dateText := strings.TrimSpace(info.Find(`.entry-date`).First().Text())
		if dateText == "" && apiScene.PublishedDate != "" {
			dateText = strings.Split(apiScene.PublishedDate, " ")[0]
		}
		if dateText != "" {
			sc.Released = parseNADate(dateText)
		}

		// Duration
		durText := strings.TrimSpace(info.Find(`.duration`).First().Text())
		sc.Duration = parseNADuration(durText)
		if sc.Duration == 0 && apiScene.Length > 0 {
			sc.Duration = apiScene.Length / 60
		}

		// Description
		synopsis := strings.TrimSpace(info.Find(`#video-description`).First().Text())
		synopsis = strings.TrimPrefix(synopsis, "Synopsis:")
		sc.Synopsis = strings.TrimSpace(synopsis)
		if sc.Synopsis == "" {
			sc.Synopsis = strings.TrimSpace(apiScene.Synopsis)
		}

		// Cast from performer list items
		sc.ActorDetails = make(map[string]models.ActorDetails)
		added := make(map[string]bool)
		tryCast := func(name, profileURL string) {
			name = strings.TrimSpace(name)
			if name == "" || added[name] {
				return
			}
			added[name] = true
			sc.Cast = append(sc.Cast, name)
			sc.ActorDetails[name] = models.ActorDetails{Source: scraperID + " scrape", ProfileUrl: strings.SplitN(profileURL, "?", 2)[0]}
		}
		info.Find(`.performer-list a`).Each(func(_ int, a *goquery.Selection) {
			href, _ := a.Attr("href")
			tryCast(a.Text(), e.Request.AbsoluteURL(href))
		})
		// Fallback to API performers
		if len(sc.Cast) == 0 {
			for _, names := range apiScene.Performers {
				for _, name := range names {
					tryCast(name, "")
				}
			}
		}

		// Tags from API
		sc.Tags = apiScene.Tags

		// Scene image: the dl8-video poster attribute is set via JS at runtime and is not
		// present in the static HTML, so use the static data-poster attribute instead.
		imageURL := e.ChildAttr(`#scene-end-cta-wrapper`, "data-poster")
		if imageURL == "" {
			imageURL = e.ChildAttr(`.dl8-embed-container dl8-video`, "poster")
		}
		if imageURL == "" {
			imageURL = e.ChildAttr(`.dl8-embed-container img`, "src")
		}
		if imageURL == "" {
			if m := regexp.MustCompile(`url\(["']?(.*?)["']?\)`).FindStringSubmatch(e.ChildAttr(`.dl8-embed-container`, "style")); m != nil {
				imageURL = m[1]
			}
		}
		if strings.HasPrefix(imageURL, "//") {
			imageURL = "https:" + imageURL
		}

		// Skip scene if we could not extract the minimum required data
		if sc.Title == "" || imageURL == "" {
			return
		}

		sc.Covers, sc.Gallery = buildNACovers(imageURL)

		// Filenames
		prefix, slug := getNABasePrefixSlug(imageURL)
		if prefix != "" && slug != "" {
			baseName := prefix + slug
			defaultBaseName := "nam" + slug
			filenames := []string{"_180x180_3dh.mp4", "_smartphonevr60.mp4", "_smartphonevr30.mp4", "_vrdesktopsd.mp4", "_vrdesktophd.mp4", "_180_sbs.mp4", "_6kvr264.mp4", "_6kvr265.mp4", "_8kvr265.mp4"}
			for i := range filenames {
				sc.Filenames = append(sc.Filenames, baseName+filenames[i], defaultBaseName+filenames[i])
			}
		}

		// Trailer
		sc.TrailerType = "heresphere"
		params := models.TrailerScrape{SceneUrl: "https://api.naughtyapi.com/heresphere/" + sc.SiteID}
		strParams, _ := json.Marshal(params)
		sc.TrailerSrc = string(strParams)

		out <- sc
	})

	if singleSceneURL != "" {
		sceneCollector.Visit(singleSceneURL)
	} else {
		apiScenes, err := fetchNAVRPages(limitScraping)
		if len(apiScenes) == 0 {
			log.Errorf("NAVR API list failed or returned no scenes, falling back to HTML listing: %v", err)

			siteCollector := createCollector("www.naughtyamerica.com")
			maxPage := 0

			// Parse pagination, including page numbers from hrefs
			siteCollector.OnHTML(`ul.pagination`, func(e *colly.HTMLElement) {
				if maxPage > 0 {
					return
				}
				e.ForEach(`li a`, func(_ int, el *colly.HTMLElement) {
					if n, err := strconv.Atoi(strings.TrimSpace(el.Text)); err == nil && n > maxPage {
						maxPage = n
					}
					if m := regexp.MustCompile(`[?&]page=(\d+)`).FindStringSubmatch(el.Attr("href")); m != nil {
						if n, err := strconv.Atoi(m[1]); err == nil && n > maxPage {
							maxPage = n
						}
					}
				})
				if maxPage > 1 && !limitScraping {
					for page := 2; page <= maxPage; page++ {
						siteCollector.Visit("https://www.naughtyamerica.com/vr-porn?page=" + strconv.Itoa(page))
					}
				}
			})

			// Scene cards on list pages
			siteCollector.OnHTML(`div.scene-item`, func(e *colly.HTMLElement) {
				sceneLink := e.ChildAttr("a.contain-img.vr-scene-item", "href")
				if sceneLink == "" {
					return
				}
				sceneURL := e.Request.AbsoluteURL(sceneLink)
				if funk.ContainsString(knownScenes, strings.Split(sceneURL, "?")[0]) {
					return
				}
				sceneCollector.Visit(sceneURL)
			})

			siteCollector.Visit("https://www.naughtyamerica.com/vr-porn")
		} else {
			for _, s := range apiScenes {
				sceneURL := strings.Split(s.SceneURL, "?")[0]
				if funk.ContainsString(knownScenes, sceneURL) {
					continue
				}
				apiJSON, _ := json.Marshal(s)
				ctx := colly.NewContext()
				ctx.Put("apiScene", string(apiJSON))
				sceneCollector.Request("GET", sceneURL, nil, ctx, nil)
			}
		}
	}

	if updateSite {
		updateSiteLastUpdate(scraperID)
	}
	logScrapeFinished(scraperID, siteID)
	return nil
}

func init() {
	registerScraper("naughtyamericavr", "NaughtyAmerica VR", "https://mcdn.vrporn.com/files/20170718100937/naughtyamericavr-vr-porn-studio-vrporn.com-virtual-reality.png", "naughtyamerica.com", NaughtyAmericaVR)
}
