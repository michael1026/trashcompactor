# TrashCompactor
Simple Go program to remove URLs with duplicate funcionality based on script resources included. The theory behind this being if two pages include the same five scripts, they most likely have the same functionality.

This tool now supports JSON as well. Uniqueness of JSON responses are based on what keys are present.

Because of how the tool works, it will only return pages with a content type of text/html or application/json and a response code of 200. 

### Installation
`go get github.com/michael1026/trashcompactor`

### Usage
`cat URLs | trashcompactor`

