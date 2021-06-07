# TrashCompactor
Simple Go program to remove URLs with duplicate funcionality based on script resources included. The theory behind this being if two pages include the same five scripts, they most likely have the same functionality.

Because of how the tool works, it will only return pages with a content type of text/html and a response code of 200. 

### Installation
`go get github.com/michael1026/TrashCompactor`

### Usage
`cat URLs | trashcompactor`
