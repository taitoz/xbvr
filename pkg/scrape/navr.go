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

// naFlexibleStringMap unmarshals a JSON object into a map, but also tolerates
// the API returning an empty array for the same field.
type naFlexibleStringMap map[string]string

// naFlexiblePerformers unmarshals performers which can be either
// map[string]string (single performer per role) or map[string][]string
// (multiple performers per role).
type naFlexiblePerformers map[string][]string

func (p *naFlexiblePerformers) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" || (len(data) > 0 && data[0] == '[') {
		*p = nil
		return nil
	}
	// Try map[string][]string first.
	var multi map[string][]string
	if err := json.Unmarshal(data, &multi); err == nil {
		*p = multi
		return nil
	}
	// Fall back to map[string]string (single performer per role).
	var single map[string]string
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	result := make(naFlexiblePerformers, len(single))
	for role, name := range single {
		result[role] = []string{name}
	}
	*p = result
	return nil
}

func (m *naFlexibleStringMap) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" || (len(data) > 0 && data[0] == '[') {
		*m = nil
		return nil
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = raw
	return nil
}

type naSceneAPI struct {
	ID             int                 `json:"id"`
	Title          string              `json:"title"`
	Length         int                 `json:"length"`
	PublishedDate  string              `json:"published_date"`
	SceneURL       string              `json:"scene_url"`
	SiteName       string              `json:"site_name"`
	Synopsis       string              `json:"synopsis"`
	Tags           []string            `json:"tags"`
	Performers     naFlexiblePerformers `json:"performers"`
	PromoVideoData naFlexibleStringMap `json:"promo_video_data"`
	Trailers       naFlexibleStringMap `json:"trailers"`
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

// fetchNAVRSceneByID fetches a single scene from the API using the scene's
// numeric ID (the trailing number in the scene URL slug).
func fetchNAVRSceneByID(id string) (naSceneAPI, bool) {
	var r naScenesResponse
	u := fmt.Sprintf("https://api.naughtyapi.com/tools/scenes/scenes?id=%s", id)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return naSceneAPI{}, false
	}
	req.Header.Set("User-Agent", naUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://www.naughtyamerica.com/")
	resp, err := naHTTPClient.Do(req)
	if err != nil {
		return naSceneAPI{}, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return naSceneAPI{}, false
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return naSceneAPI{}, false
	}
	for _, s := range r.Data {
		if strconv.Itoa(s.ID) == id {
			return s, true
		}
	}
	return naSceneAPI{}, false
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

// getNAVRImageURL derives the static scene cover URL from promo/trailer URLs.
// The promo URL path contains the site code and the image slug directly; trailer
// URLs contain the site code plus the same slug prefixed with the site code.
func getNAVRImageURL(apiScene naSceneAPI) string {
	// promo_video_data URLs: .../public/promo/{site}/{imageSlug}/{filename}.mp4
	for _, u := range apiScene.PromoVideoData {
		parts := strings.Split(u, "/")
		if len(parts) >= 3 {
			site := parts[len(parts)-3]
			slug := parts[len(parts)-2]
			if site != "" && slug != "" {
				return fmt.Sprintf("https://images1.naughtycdn.com/cms/nacmscontent/v1/scenes/%s/%s/scene/horizontal/1279x852c.jpg", site, slug)
			}
		}
	}

	// trailer URLs: .../nonsecure/{site}/trailers/vr/{site}{imageSlug}/{site}{imageSlug}teaser_...mp4
	for _, u := range apiScene.Trailers {
		parts := strings.Split(u, "/")
		if len(parts) >= 9 {
			site := parts[4]
			slug := parts[7]
			imageSlug := strings.TrimPrefix(slug, site)
			if site != "" && imageSlug != "" {
				return fmt.Sprintf("https://images1.naughtycdn.com/cms/nacmscontent/v1/scenes/%s/%s/scene/horizontal/1279x852c.jpg", site, imageSlug)
			}
		}
	}

	return ""
}

// processNAVRScene builds and emits a ScrapedScene directly from the API data,
// avoiding the WAF-protected scene detail pages.
func processNAVRScene(apiScene naSceneAPI, out chan<- models.ScrapedScene, scraperID, siteID string) {
	sc := models.ScrapedScene{}
	sc.ScraperID = scraperID
	sc.SceneType = "VR"
	sc.Studio = "NaughtyAmerica"
	sc.Site = siteID
	sc.HomepageURL = strings.Split(apiScene.SceneURL, "?")[0]
	sc.MembersUrl = strings.Replace(sc.HomepageURL, "https://www.naughtyamerica.com/", "https://members.naughtyamerica.com/", 1)
	sc.SiteID = getNaughtyAmericaSceneID(sc.HomepageURL)
	sc.SceneID = slugify.Slugify(sc.Site) + "-" + sc.SiteID

	siteTitle := strings.TrimSpace(apiScene.SiteName)
	sceneTitle := strings.TrimSpace(apiScene.Title)
	if siteTitle != "" && sceneTitle != "" {
		sc.Title = siteTitle + " " + sceneTitle
	} else if sceneTitle != "" {
		sc.Title = sceneTitle
	} else if siteTitle != "" {
		sc.Title = siteTitle
	}

	if apiScene.PublishedDate != "" {
		sc.Released = parseNADate(strings.Split(apiScene.PublishedDate, " ")[0])
	}

	if apiScene.Length > 0 {
		sc.Duration = apiScene.Length / 60
	}

	sc.Synopsis = strings.TrimSpace(apiScene.Synopsis)

	sc.ActorDetails = make(map[string]models.ActorDetails)
	added := make(map[string]bool)
	for _, names := range apiScene.Performers {
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" || added[name] {
				continue
			}
			added[name] = true
			sc.Cast = append(sc.Cast, name)
			sc.ActorDetails[name] = models.ActorDetails{
				Source:     scraperID + " scrape",
				ProfileUrl: "https://www.naughtyamerica.com/pornstar/" + slugify.Slugify(name),
			}
		}
	}

	sc.Tags = apiScene.Tags

	imageURL := getNAVRImageURL(apiScene)
	if imageURL == "" {
		log.Warnf("NAVR: could not derive cover image for scene %s", sc.HomepageURL)
		return
	}
	if strings.HasPrefix(imageURL, "//") {
		imageURL = "https:" + imageURL
	}

	sc.Covers, sc.Gallery = buildNACovers(imageURL)

	prefix, slug := getNABasePrefixSlug(imageURL)
	if prefix != "" && slug != "" {
		baseName := prefix + slug
		defaultBaseName := "nam" + slug
		filenames := []string{"_180x180_3dh.mp4", "_smartphonevr60.mp4", "_smartphonevr30.mp4", "_vrdesktopsd.mp4", "_vrdesktophd.mp4", "_180_sbs.mp4", "_6kvr264.mp4", "_6kvr265.mp4", "_8kvr265.mp4"}
		for i := range filenames {
			sc.Filenames = append(sc.Filenames, baseName+filenames[i], defaultBaseName+filenames[i])
		}
	}

	sc.TrailerType = "heresphere"
	params := models.TrailerScrape{SceneUrl: "https://api.naughtyapi.com/heresphere/" + sc.SiteID}
	strParams, _ := json.Marshal(params)
	sc.TrailerSrc = string(strParams)

	out <- sc
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
		if imageURL == "" {
			imageURL = e.ChildAttr(`meta[property="og:image"]`, "content")
		}
		if strings.HasPrefix(imageURL, "//") {
			imageURL = "https:" + imageURL
		}

		// Skip scene if we could not extract the minimum required data
		if sc.Title == "" || imageURL == "" {
			log.Warnf("NAVR: skipping scene %s (title=%q image=%q)", sc.HomepageURL, sc.Title, imageURL)
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
		// Fetch scene data from the API to bypass WAF on the HTML page.
		singleSiteID := getNaughtyAmericaSceneID(strings.Split(singleSceneURL, "?")[0])
		found := false
		// Try direct lookup by scene ID first (scenes API supports ?id= filter).
		if apiScene, ok := fetchNAVRSceneByID(singleSiteID); ok {
			processNAVRScene(apiScene, out, scraperID, siteID)
			found = true
		}
		if !found {
			// Fall back: walk pages from the end (newest first) to find the scene.
			first, err := fetchNAVRPage(1)
			if err == nil && first.LastPage > 0 {
				for page := first.LastPage; page >= 1; page-- {
					r, err := fetchNAVRPage(page)
					if err != nil || len(r.Data) == 0 {
						continue
					}
					for _, s := range r.Data {
						if getNaughtyAmericaSceneID(strings.Split(s.SceneURL, "?")[0]) == singleSiteID {
							processNAVRScene(s, out, scraperID, siteID)
							found = true
							break
						}
					}
					if found {
						break
					}
				}
			}
		}
		if !found {
			log.Warnf("NAVR: scene %s not found in API", singleSceneURL)
		}
	} else {
		apiScenes, err := fetchNAVRPages(limitScraping)
		knownCount := 0
		for _, s := range apiScenes {
			if funk.ContainsString(knownScenes, strings.Split(s.SceneURL, "?")[0]) {
				knownCount++
			}
		}
		log.Infof("NAVR: API returned %d scenes, %d already known, %d to visit", len(apiScenes), knownCount, len(apiScenes)-knownCount)
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
			newCount := 0
			for _, s := range apiScenes {
				sceneURL := strings.Split(s.SceneURL, "?")[0]
				if funk.ContainsString(knownScenes, sceneURL) {
					continue
				}
				newCount++
				processNAVRScene(s, out, scraperID, siteID)
			}
			log.Infof("NAVR: processed %d new scenes from API", newCount)
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
