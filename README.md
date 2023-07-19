# contester

## Setup:

1. Clone the repository
2. Install dependencies: `go install`
3. Create a file named `.env`. It should have one line, of the format `API_KEY=something`. Obtain the API key from https://freeimage.host/page/api.
4. Setup Google Workspace APIs. Go to https://console.cloud.google.com and create a new project. You will need Google Drive, Google Sheets and Google Forms APIs turned on, you can do this by going into More Products > Other Google Products > Google Workspace > Product Library. Also, set up the OAuth consent screen under APIs & Services. If you get some weird unspecific error during this, you may need to configure the Firebase developer email for the project. I have no idea why.
5. Once this is set up, Go to APIs & Services > Credentials and download the OAuth client you just created. Rename it to `credentials.json` and put it in this folder.
6. Run the program: `go run main.go [pokemon name] [contest number] "Category 1" "Category 2" "Category 3"`
7. On the first run, it'll ask you to get an authorization code from the link. Click it, log in, click "Advanced" and then click "Go to (project name) (unsafe)", then click Continue. Now, copy the URL of the localhost page you are now on. Specifically, you want to copy the code, which is the string of characters between `code=` and `&scope`. Paste this back into the console and press enter.
8. Enjoy!

## Limitations:

1. Form banners cannot be set with the API
2. Only limited image formats are supported
3. Checkbox limit cannot be set with the API