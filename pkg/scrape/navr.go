package scrape

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gocolly/colly/v2"
	"github.com/mozillazg/go-slugify"
	"github.com/nleeper/goment"
	"github.com/thoas/go-funk"
	"github.com/xbapps/xbvr/pkg/models"
)

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
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
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

func getNaughtyAmericaSceneID(sceneURL string) string {
	tmp := strings.Split(sceneURL, "-")
	return tmp[len(tmp)-1]
}

func NaughtyAmericaVR(wg *models.ScrapeWG, updateSite bool, knownScenes []string, out chan<- models.ScrapedScene, singleSceneURL string, singeScrapeAdditionalInfo string, limitScraping bool) error {
	defer wg.Done()
	scraperID := "naughtyamericavr"
	siteID := "NaughtyAmerica VR"
	logScrapeStart(scraperID, siteID)

	siteCollector := createCollector("www.naughtyamerica.com")

	processScene := func(sceneURL string, listCard *colly.HTMLElement) bool {
		sceneURL = strings.Split(sceneURL, "?")[0]
		sceneID := getNaughtyAmericaSceneID(sceneURL)

		apiScene, ok := fetchNaughtyAmericaScene(sceneID)
		if !ok {
			return false
		}

		sc := models.ScrapedScene{}
		sc.ScraperID = scraperID
		sc.SceneType = "VR"
		sc.Studio = "NaughtyAmerica"
		sc.Site = siteID
		sc.HomepageURL = sceneURL
		sc.MembersUrl = strings.Replace(sceneURL, "https://www.naughtyamerica.com/", "https://members.naughtyamerica.com/", 1)
		sc.SiteID = sceneID
		sc.SceneID = slugify.Slugify(sc.Site) + "-" + sc.SiteID

		// Title: site title + scene title when available from list card
		if listCard != nil {
			siteTitle := strings.TrimSpace(listCard.ChildText("a.site-title"))
			if siteTitle != "" && apiScene.Title != "" {
				sc.Title = siteTitle + " - " + apiScene.Title
			} else {
				sc.Title = apiScene.Title
			}
		} else {
			sc.Title = apiScene.Title
		}

		// Date
		if listCard != nil {
			dateText := strings.TrimSpace(listCard.ChildText("div.entry-date p.light-grey-text"))
			if dateText != "" {
				tmpDate, _ := goment.New(dateText, "MMM DD, YYYY")
				sc.Released = tmpDate.Format("YYYY-MM-DD")
			}
		}

		// Duration (API returns milliseconds)
		if apiScene.Duration > 0 {
			sc.Duration = apiScene.Duration / 60000
		}

		sc.Synopsis = apiScene.Description

		// Tags
		for _, t := range apiScene.Tags {
			sc.Tags = append(sc.Tags, t.Name)
		}

		// Cast from list card
		sc.ActorDetails = make(map[string]models.ActorDetails)
		if listCard != nil {
			listCard.ForEach("p.contain-actors a.title", func(id int, e *colly.HTMLElement) {
				name := strings.TrimSpace(e.Text)
				if name == "" {
					return
				}
				sc.Cast = append(sc.Cast, name)
				sc.ActorDetails[name] = models.ActorDetails{Source: scraperID + " scrape", ProfileUrl: strings.SplitN(e.Request.AbsoluteURL(e.Attr("href")), "?", 2)[0]}
			})
		}

		// Trailer
		sc.TrailerType = "heresphere"
		params := models.TrailerScrape{SceneUrl: "https://api.naughtyapi.com/heresphere/" + sc.SiteID}
		strParams, _ := json.Marshal(params)
		sc.TrailerSrc = string(strParams)

		// Covers & filenames from list card image
		if listCard != nil {
			vertWebp := listCard.ChildAttr("a.contain-img.vr-scene-item picture source[type='image/webp']", "srcset")
			if vertWebp != "" {
				vertWebp = listCard.Request.AbsoluteURL(strings.Split(vertWebp, "?")[0])

				// horizontal webp
				sc.Covers = append(sc.Covers, strings.Replace(vertWebp, "/vertical/1182x1788c.webp", "/horizontal/1182x777c.webp", 1))
				// vertical webp
				sc.Covers = append(sc.Covers, vertWebp)
				// horizontal jpg
				sc.Covers = append(sc.Covers, strings.Replace(vertWebp, "/vertical/1182x1788c.webp", "/horizontal/1182x777c.jpg", 1))
				// vertical jpg
				sc.Covers = append(sc.Covers, strings.Replace(vertWebp, "/vertical/1182x1788c.webp", "/vertical/1182x1788c.jpg", 1))

				base := strings.Split(strings.Replace(vertWebp, "//", "", -1), "/")
				if len(base) >= 7 {
					baseName := base[5] + base[6]
					defaultBaseName := "nam" + base[6]
					filenames := []string{"_180x180_3dh.mp4", "_smartphonevr60.mp4", "_smartphonevr30.mp4", "_vrdesktopsd.mp4", "_vrdesktophd.mp4", "_180_sbs.mp4", "_6kvr264.mp4", "_6kvr265.mp4", "_8kvr265.mp4"}
					for i := range filenames {
						sc.Filenames = append(sc.Filenames, baseName+filenames[i], defaultBaseName+filenames[i])
					}
				}
			}
		} else {
			// Single scene fallback: use API thumbnail and construct webp variants
			if apiScene.ThumbnailImage != "" {
				thumb := strings.Split(apiScene.ThumbnailImage, "?")[0]
				sc.Covers = append(sc.Covers, thumb)
				sc.Covers = append(sc.Covers, strings.Replace(thumb, ".jpg", ".webp", 1))
			}
		}

		out <- sc
		return true
	}

	// Pagination
	siteCollector.OnHTML(`ul.pagination li a[rel="next"]`, func(e *colly.HTMLElement) {
		if !limitScraping {
			siteCollector.Visit(e.Request.AbsoluteURL(e.Attr("href")))
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

		processScene(sceneURL, e)
	})

	if singleSceneURL != "" {
		processScene(singleSceneURL, nil)
	} else {
		siteCollector.Visit("https://www.naughtyamerica.com/vr-porn")
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
