package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/forms/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func upload(client *http.Client, url string, values map[string]io.Reader) (resp string, err error) {
	// Prepare a form that you will submit to that URL.
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for key, r := range values {
		var fw io.Writer
		if x, ok := r.(io.Closer); ok {
			defer x.Close()
		}
		// Add an image file
		if x, ok := r.(*os.File); ok {
			if fw, err = w.CreateFormFile(key, x.Name()); err != nil {
				return
			}
		} else {
			// Add other fields
			if fw, err = w.CreateFormField(key); err != nil {
				return
			}
		}
		if _, err = io.Copy(fw, r); err != nil {
			return "", err
		}

	}
	// Don't forget to close the multipart writer.
	// If you don't close it, your request will be missing the terminating boundary.
	w.Close()

	// Now that you have a form, you can submit it to your handler.
	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return
	}
	// Don't forget to set the content type, this will contain the boundary.
	req.Header.Set("Content-Type", w.FormDataContentType())

	// Submit the request
	res, err := client.Do(req)
	if err != nil {
		return
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(res.Body)
	respBytes := buf.String()

	respString := string(respBytes)

	// Check the response
	if res.StatusCode != http.StatusOK {
		err = fmt.Errorf("bad status: %s", res.Status)
	}
	return respString, err
}

func mustOpen(f string) *os.File {
	r, err := os.Open(f)
	if err != nil {
		panic(err)
	}
	return r
}

func main() {
	// parse input
	pkmn := os.Args[1]
	number, err := strconv.Atoi(os.Args[2])
	categories := os.Args[3:]
	if err != nil {
		log.Fatalf("Unable to parse number: %v", err)
	}
	// fmt.Println(pkmn, number)

	// img api
	err = godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Unable to load .env file: %v", err)
	}
	key := os.Getenv("API_KEY")
	posturl := fmt.Sprintf("https://freeimage.host/api/1/upload?key=%s&action=upload", key)

	// read credentials
	ctx := context.Background()
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	// Create clients
	drivesrv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Drive client: %v", err)
	}
	sheetsrv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Sheets client: %v", err)
	}
	formsrv, err := forms.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Forms client: %v", err)
	}

	// Locate spreadsheet
	sheetFile, err := drivesrv.Files.List().Q(fmt.Sprintf("name contains '%d (Responses)' and not name contains 'File' or name contains '%d (Risposte)' and not name contains 'File'", number, number)).PageSize(10).Fields("nextPageToken, files(id, name)").Do()
	// fmt.Println(r, err)
	if err != nil {
		log.Fatalf("Unable to find form responses sheet (was it created from the form?): %v", err)
	}
	sheetId := sheetFile.Files[0].Id
	sheet, err := sheetsrv.Spreadsheets.Get(sheetId).Do()
	if err != nil {
		log.Fatalf("Unable to get spreadsheet from file: %v", err)
	}
	// fmt.Println(sheet.Sheets[0].Properties.Title)
	readRange := sheet.Sheets[0].Properties.Title + "!B2:D" // read username/unsigned link responses
	submissions, err := sheetsrv.Spreadsheets.Values.Get(sheetId, readRange).Do()
	if err != nil {
		log.Fatalf("Unable to retrieve data from sheet: %v", err)
	}

	for _, row := range submissions.Values {
		// Print each submission
		fmt.Printf("%s, %s\n", row[0], row[2])
	}

	// find contest folder
	folder, err := drivesrv.Files.List().Q(fmt.Sprintf("name contains 'Contest #%d' and not name contains 'Art'", number)).PageSize(10).Fields("nextPageToken, files(id, name)").Do()
	if err != nil {
		log.Fatalf("Unable to retrieve files: %v", err)
	}

	// create form
	// fmt.Println(formsrv)
	formsrv.Forms.Create(&forms.Form{Info: &forms.Info{Title: fmt.Sprintf("Pokémon Workshop Art Contest #%d %s Voting", number, pkmn), DocumentTitle: fmt.Sprintf("Pokémon Workshop Art Contest #%d %s Voting", number, pkmn)}}).Do()
	form, err := drivesrv.Files.List().Q(fmt.Sprintf("name contains 'Art Contest #%d Voting'", number)).PageSize(10).Fields("nextPageToken, files(id, name)").Do()
	if err != nil {
		log.Fatalf("Unable to retrieve files: %v", err)
	}
	_, err = drivesrv.Files.Update(form.Files[0].Id, &drive.File{}).RemoveParents("root").AddParents(folder.Files[0].Id).Do()
	if err != nil {
		log.Fatalf("Unable to move file: %v", err)
	}

	// form sections 1-2
	_, err = formsrv.Forms.BatchUpdate(form.Files[0].Id, &forms.BatchUpdateFormRequest{
		Requests: []*forms.Request{
			{
				CreateItem: &forms.CreateItemRequest{
					Item: &forms.Item{
						Title: "Discord Username",
						QuestionItem: &forms.QuestionItem{
							Question: &forms.Question{
								Required: true,
								TextQuestion: &forms.TextQuestion{
									Paragraph: false,
								},
							},
						},
					},
					Location: &forms.Location{
						Index:           0,
						ForceSendFields: []string{"Index"},
					},
				},
			},
			{
				CreateItem: &forms.CreateItemRequest{
					Item: &forms.Item{
						Title:         "The Submissions!",
						Description:   "You will be presented with all the submissions first. You will be able to vote on them after viewing all of them.",
						PageBreakItem: &forms.PageBreakItem{},
					},
					Location: &forms.Location{
						Index: 1,
					},
				},
			},
		},
	}).Do()
	if err != nil {
		log.Fatalf("Unable to update form: %v", err)
	}

	var thumbs []string
	var links []string

	// randomize order
	for i := range submissions.Values {
		j := rand.Intn(i + 1)
		submissions.Values[i], submissions.Values[j] = submissions.Values[j], submissions.Values[i]
	}

	// insert images in form
	for i, row := range submissions.Values {
		img, err := drivesrv.Files.Get(strings.Split(fmt.Sprintf("%s", row[2]), "=")[1]).Fields("webContentLink, thumbnailLink").Do()
		if err != nil {
			log.Fatalf("Unable to get image: %v", err)
		}
		thumbs = append(thumbs, img.ThumbnailLink)
		// fmt.Println(img.ThumbnailLink)
		actualImg, err := drivesrv.Files.Get(strings.Split(fmt.Sprintf("%s", row[2]), "=")[1]).Download()
		// fmt.Println(actualImg)
		if err != nil {
			log.Fatalf("Unable to download image: %v", err)
		}
		defer actualImg.Body.Close()

		// Open destination file
		var flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC

		f, err := os.OpenFile("img", flags, 0644)
		if err != nil {
			log.Fatalf("Unable to open file: %v", err)
		}
		defer f.Close()

		// Download file data
		_, err = io.Copy(io.MultiWriter(f), actualImg.Body)
		if err != nil {
			log.Fatalf("Unable to write file: %v", err)
		}

		// var client *http.Client
		client := &http.Client{Timeout: time.Duration(1000) * time.Second}
		// var remoteURL string
		// {
		// 	//setup a mocked http client.
		// 	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 		b, err := httputil.DumpRequest(r, true)
		// 		if err != nil {
		// 			panic(err)
		// 		}
		// 		fmt.Printf("%s", b)
		// 	}))
		// 	defer ts.Close()
		// 	client = ts.Client()
		// 	remoteURL = ts.URL
		// }

		// prepare the reader instances to encode
		values := map[string]io.Reader{
			"source": mustOpen("img"), // lets assume its this file
			"key":    strings.NewReader(key),
			"action": strings.NewReader("upload"),
		}
		resp, err := upload(client, posturl, values)
		url := "https://picsum.photos/800"
		if err != nil {
			// log.Fatalf("Unable to upload file: %v", err)
			// upload filler to avoid location collision
		} else {
			bytes := []byte(resp)
			turl, err := jsonparser.GetString(bytes, "image", "url")
			if err != nil {
				log.Fatalf("Unable to parse response: %v", err)
			}
			url = turl
		}

		// request, _ := http.NewRequest("PUT", myURL, file)
		// ^^^ why does this not work???

		// var buf bytes.Buffer
		// tee := io.TeeReader(file, &buf)
		// ioutil.ReadAll(tee)
		// request, _ := http.NewRequest("POST", posturl, &buf) // this works fine

		// request.Header.Set("Content-Type", "multipart/form-data")
		// res, err := http.DefaultClient.Do(request)
		// fmt.Println(res.Body, err)

		// bimg, err := imgbase64.FromLocal("img.png")
		// if err != nil {
		// 	log.Fatalf("Unable to read file: %v", err)
		// }

		// params := url.Values{}
		// params.Add("key", key)
		// params.Add("action", "upload")
		// params.Add("source", bimg)

		// res, err := http.PostForm(posturl, params)
		// if err != nil {
		// 	panic(err)
		// }
		// defer res.Body.Close()
		// fmt.Println(bimg)
		// fmt.Println(res)

		_, err = formsrv.Forms.BatchUpdate(form.Files[0].Id, &forms.BatchUpdateFormRequest{
			Requests: []*forms.Request{
				{
					CreateItem: &forms.CreateItemRequest{
						Item: &forms.Item{
							Title:         fmt.Sprintf("Option %d", i+1),
							PageBreakItem: &forms.PageBreakItem{},
						},
						Location: &forms.Location{
							Index: int64(i*2 + 2),
						},
					},
				},
				{
					CreateItem: &forms.CreateItemRequest{
						Item: &forms.Item{
							ImageItem: &forms.ImageItem{
								Image: &forms.Image{
									SourceUri: url,
								},
							},
						},
						Location: &forms.Location{
							Index: int64(i*2 + 3),
						},
					},
				},
			},
		}).Do()
		if err != nil {
			log.Fatalf("Unable to update form: %v", err)
		}
		links = append(links, url)
	}

	// add voting questions
	location := len(submissions.Values)*2 + 2
	fmt.Println(links, thumbs)
	options := []*forms.Option{}
	for i, link := range thumbs {
		options = append(options, &forms.Option{
			Value: fmt.Sprintf("Option %d", i+1),
			Image: &forms.Image{
				SourceUri: link,
			},
		})
	}
	for _, cat := range categories {
		_, err = formsrv.Forms.BatchUpdate(form.Files[0].Id, &forms.BatchUpdateFormRequest{
			Requests: []*forms.Request{
				{
					CreateItem: &forms.CreateItemRequest{
						Item: &forms.Item{
							Title:         cat,
							PageBreakItem: &forms.PageBreakItem{},
						},
						Location: &forms.Location{
							Index: int64(location),
						},
					},
				},
				{
					CreateItem: &forms.CreateItemRequest{
						Item: &forms.Item{
							Title: fmt.Sprintf("Which entries fit \"%s\" best? Vote up to %d! (Please remember to try to vote for different entries for each category!)", cat, 3),
							QuestionItem: &forms.QuestionItem{
								Question: &forms.Question{
									Required: true,
									ChoiceQuestion: &forms.ChoiceQuestion{
										Type:    "CHECKBOX",
										Options: options,
										Shuffle: false,
									},
								},
							},
						},
						Location: &forms.Location{
							Index: int64(location + 1),
						},
					},
				},
			},
		}).Do()
		if err != nil {
			log.Fatalf("Unable to update form: %v", err)
		}
		location += 2
	}
	// wildcard and feedback
	_, err = formsrv.Forms.BatchUpdate(form.Files[0].Id, &forms.BatchUpdateFormRequest{
		Requests: []*forms.Request{
			{
				CreateItem: &forms.CreateItemRequest{
					Item: &forms.Item{
						Title:         "Wildcard",
						PageBreakItem: &forms.PageBreakItem{},
					},
					Location: &forms.Location{
						Index: int64(location),
					},
				},
			},
			{
				CreateItem: &forms.CreateItemRequest{
					Item: &forms.Item{
						Title: fmt.Sprintf("Which entries do you feel are worthy of recognition? Vote up to %d! (Please remember to try to vote for different entries for each category!)", 3),
						QuestionItem: &forms.QuestionItem{
							Question: &forms.Question{
								Required: true,
								ChoiceQuestion: &forms.ChoiceQuestion{
									Type:    "CHECKBOX",
									Options: options,
									Shuffle: false,
								},
							},
						},
					},
					Location: &forms.Location{
						Index: int64(location + 1),
					},
				},
			},
			{
				CreateItem: &forms.CreateItemRequest{
					Item: &forms.Item{
						Title:         "Feedback",
						Description:   "We want these contests to be the best they can be. Please tell us any suggestions/concerns you may have!",
						PageBreakItem: &forms.PageBreakItem{},
					},
					Location: &forms.Location{
						Index: int64(location + 2),
					},
				},
			},
			{
				CreateItem: &forms.CreateItemRequest{
					Item: &forms.Item{
						Title: "Optional Feedback",
						QuestionItem: &forms.QuestionItem{
							Question: &forms.Question{
								Required: false,
								TextQuestion: &forms.TextQuestion{
									Paragraph: true,
								},
							},
						},
					},
					Location: &forms.Location{
						Index: int64(location + 3),
					},
				},
			},
		},
	}).Do()
	if err != nil {
		log.Fatalf("Unable to update form: %v", err)
	}
	location += 4

	/*
		_, err = formsrv.Forms.BatchUpdate(form.Files[0].Id, &forms.BatchUpdateFormRequest{Requests: []*forms.Request{{UpdateFormInfo: &forms.UpdateFormInfoRequest{Info: &forms.Info{Description: "This is a description"}, UpdateMask: "description"}}}}).Do()
		if err != nil {
			log.Fatalf("Unable to update description: %v", err)
		}
	*/

	// fmt.Println("Files:")
	// if len(folder.Files) == 0 {
	// 	fmt.Println("No files found.")
	// } else {
	// 	for _, i := range folder.Files {
	// 		fmt.Printf("%s (%s)\n", i.Name, i.Id)
	// 	}
	// }

}
