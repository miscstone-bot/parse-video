package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/tidwall/gjson"
	"github.com/wujunwei928/parse-video/utils"
)

type douYin struct{}

func (d douYin) parseVideoID(videoId string) (*VideoParseInfo, error) {
	reqUrl := fmt.Sprintf("https://www.iesdouyin.com/share/video/%s", videoId)

	client := resty.New()
	res, err := client.R().
		SetHeader(HttpHeaderUserAgent, "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1 Edg/122.0.0.0").
		Get(reqUrl)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`window._ROUTER_DATA\s*=\s*(.*?)</script>`)
	findRes := re.FindSubmatch(res.Body())
	if len(findRes) < 2 {
		return nil, errors.New("parse video json info from html fail")
	}

	jsonBytes := bytes.TrimSpace(findRes[1])
	data := gjson.GetBytes(jsonBytes, "loaderData.video_(id)/page.videoInfoRes.item_list.0")

	if !data.Exists() {
		filterObj := gjson.GetBytes(
			jsonBytes,
			fmt.Sprintf(`loaderData.video_(id)/page.videoInfoRes.filter_list.#(aweme_id=="%s")`, videoId),
		)

		return nil, fmt.Errorf(
			"get video info fail: %s - %s",
			filterObj.Get("filter_reason"),
			filterObj.Get("detail_msg"),
		)
	}

	// 获取图集图片地址
	imagesObjArr := data.Get("images").Array()
	images := make([]string, 0, len(imagesObjArr))
	for _, imageItem := range imagesObjArr {
		imageUrl := imageItem.Get("url_list.0").String()
		if len(imageUrl) > 0 {
			images = append(images, imageUrl)
		}
	}

	// 获取视频播放地址
	videoUrl := data.Get("video.play_addr.url_list.0").String()
	videoUrl = strings.ReplaceAll(videoUrl, "playwm", "play")
	data.Get("video.play_addr.url_list").ForEach(func(key, value gjson.Result) bool {
		fmt.Println(strings.ReplaceAll(value.String(), "playwm", "play"))
		return true
	})

	// 如果图集地址不为空时，因为没有视频，上面抖音返回的视频地址无法访问，置空处理
	if len(images) > 0 {
		videoUrl = ""
	}

	videoInfo := &VideoParseInfo{
		Title:    data.Get("desc").String(),
		VideoUrl: videoUrl,
		MusicUrl: "",
		CoverUrl: data.Get("video.cover.url_list.0").String(),
		Images:   images,
	}
	videoInfo.Author.Uid = data.Get("author.sec_uid").String()
	videoInfo.Author.Name = data.Get("author.nickname").String()
	videoInfo.Author.Avatar = data.Get("author.avatar_thumb.url_list.0").String()

	// 视频地址非空时，获取302重定向之后的视频地址
	// 图集时，视频地址为空，不处理
	if len(videoInfo.VideoUrl) > 0 {
		d.getRedirectUrl(videoInfo)
	}

	liveImagesUrl, err := d.parseNoteLiveImages(res.Body())
	if err != nil {
		log.Printf("parseNoteLiveImages failed: err=%v", err)
	}

	if len(liveImagesUrl) > 0 {
		videoInfo.LiveImagesUrl = liveImagesUrl
	}
	return videoInfo, nil
}

func (d douYin) parseShareUrl(shareUrl string) (*VideoParseInfo, error) {
	urlRes, err := url.Parse(shareUrl)
	if err != nil {
		return nil, err
	}

	switch urlRes.Host {
	case "www.iesdouyin.com", "www.douyin.com":
		return d.parsePcShareUrl(shareUrl) // 解析电脑网页端链接
	case "v.douyin.com":
		return d.parseAppShareUrl(shareUrl) // 解析App分享链接
	}

	return nil, fmt.Errorf("douyin not support this host: %s", urlRes.Host)
}

func (d douYin) parseAppShareUrl(shareUrl string) (*VideoParseInfo, error) {
	// 适配App分享链接类型:
	// https://v.douyin.com/xxxxxx/

	client := resty.New()
	// disable redirects in the HTTP client, get params before redirects
	client.SetRedirectPolicy(resty.NoRedirectPolicy())
	res, err := client.R().
		SetHeader(HttpHeaderUserAgent, DefaultUserAgent).
		Get(shareUrl)
	// 非 resty.ErrAutoRedirectDisabled 错误时，返回错误
	if !errors.Is(err, resty.ErrAutoRedirectDisabled) {
		return nil, err
	}

	locationRes, err := res.RawResponse.Location()
	if err != nil {
		return nil, err
	}

	videoId, err := d.parseVideoIdFromPath(locationRes.Path)
	if err != nil {
		return nil, err
	}
	if len(videoId) <= 0 {
		return nil, errors.New("parse video id from share url fail")
	}

	// 西瓜视频解析方式不一样
	if strings.Contains(locationRes.Host, "ixigua.com") {
		return xiGua{}.parseVideoID(videoId)
	}

	return d.parseVideoID(videoId)
}

func (d douYin) parsePcShareUrl(shareUrl string) (*VideoParseInfo, error) {
	// 适配电脑网页端链接类型
	// https://www.iesdouyin.com/share/video/xxxxxx/
	// https://www.douyin.com/video/xxxxxx
	videoId, err := d.parseVideoIdFromPath(shareUrl)
	if err != nil {
		return nil, err
	}
	return d.parseVideoID(videoId)
}

func (d douYin) parseVideoIdFromPath(urlPath string) (string, error) {
	if len(urlPath) <= 0 {
		return "", errors.New("url path is empty")
	}

	urlPath = strings.Trim(urlPath, "/")
	urlSplit := strings.Split(urlPath, "/")

	// 获取最后一个元素
	if len(urlSplit) > 0 {
		return urlSplit[len(urlSplit)-1], nil
	}

	return "", errors.New("parse video id from path fail")
}

// parseNoteData 解析douyin动态图片
func (d douYin) parseNoteLiveImages(resBody []byte) ([]string, error) {
	redirectNoteUrlRe := regexp.MustCompile(`https://www.douyin.com/note/(\d*)`)
	redirectNoteFindRes := redirectNoteUrlRe.FindSubmatch(resBody)
	if len(redirectNoteFindRes) < 2 {
		fmt.Println("parse note data skip, found no note url")
		return nil, nil
	}
	douyinNoteUrl := string(redirectNoteFindRes[0])

	cookie := fmt.Sprintf("%v=%v;%v=%v;", DouyinACNonceHeader, "06796360e00bcf7d1f696", DouyinACSignature, "_02B4Z6wo00f01hpfmEgAAIDBSKC4UcxY8gIaf5zAAOEP64")

	client := resty.New()
	res, err := client.R().
		SetHeader(HttpHeaderUserAgent, "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36 Edg/132.0.0.0").
		SetHeader(HttpHeaderCookie, cookie).
		Get(douyinNoteUrl)
	if err != nil {
		return nil, err
	}

	// Define the regex pattern to match the script content
	pattern := `<script[^>]*>(.*?)</script>`
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(string(res.Body()), -1)

	targetText := "self.__pace_f.push"
	for _, match := range matches {
		if len(match) <= 1 {
			continue
		}
		scriptContent := match[1]
		if strings.Contains(scriptContent, targetText) && strings.Contains(scriptContent, "awemeId") {
			targetRes := utils.ExtractBetweenFirstAndLastBraces(scriptContent)
			targetRes = utils.ParseJsonWithLevel(targetRes, 2)

			playApiListStr := gjson.Get(targetRes, "aweme.detail.images.#.video.playApi")

			var playApiUrls []string
			err = json.Unmarshal([]byte(playApiListStr.Raw), &playApiUrls)
			if err != nil {
				return nil, err
			}

			for idx, url := range playApiUrls {
				domainIndex := strings.Index(url, "www.douyin.com")
				if domainIndex == -1 {
					continue
				}
				playApiUrls[idx] = "https://" + url[domainIndex:]
			}
			return playApiUrls, nil
		}
	}
	return nil, nil
}

func (d douYin) getRedirectUrl(videoInfo *VideoParseInfo) {
	client := resty.New()
	client.SetRedirectPolicy(resty.NoRedirectPolicy())
	res2, _ := client.R().
		SetHeader(HttpHeaderUserAgent, DefaultUserAgent).
		Get(videoInfo.VideoUrl)
	locationRes, _ := res2.RawResponse.Location()
	if locationRes != nil {
		(*videoInfo).VideoUrl = locationRes.String()
	}
}

func (d douYin) randSeq(n int) string {
	letters := []rune("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
