# TrashCompactor
Simple Go program to remove URLs with duplicate funcionality based on script resources included. The theory behind this being if two pages include the same five scripts, they most likely have the same functionality.

This tool now supports JSON as well. Uniqueness of JSON responses are based on what keys are present.

Because of how the tool works, it will only return pages with a content type of text/html or application/json and a response code of 200. 

### Installation
`go get github.com/michael1026/trashcompactor`

### Usage
`cat URLs | trashcompactor`

### Authenticated Scanning
This tool supports scanning using cookies for multiple websites. To use this feature, create a JSON file containing URLs and their associated cookies, like the following...

```
{
    "https://www.example.com/":"session=15as51c8se1et",
    "https://www.example.org/":"jwt=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
}
```

Then use the `-C` argument to provide this file. 
