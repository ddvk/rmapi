package shell

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/abiosoft/ishell"
	"github.com/juruen/rmapi/annotations"
	"github.com/juruen/rmapi/archive"
)

func export(zipName, docName string) error {
	file, err := os.Open(zipName)
	if err != nil {
		return err
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return err
	}

	zip := archive.NewZip()
	// read file into note
	err = zip.Read(file, fi.Size())
	if err != nil {
		return err
	}

	ft := zip.Content.FileType
	if zip.Payload != nil {
		outputName := docName + "." + ft
		return ioutil.WriteFile(outputName, zip.Payload, 0666)
	}
	return nil
}

func getACmd(ctx *ShellCtxt) *ishell.Cmd {
	return &ishell.Cmd{
		Name:      "geta",
		Help:      "copy remote file to local and generate a PDF with its annotations",
		Completer: createEntryCompleter(ctx),
		Func: func(c *ishell.Context) {

			flagSet := flag.NewFlagSet("geta", flag.ContinueOnError)
			addPageNumbers := flagSet.Bool("p", false, "add page numbers")
			allPages := flagSet.Bool("a", false, "all pages")
			annotationsOnly := flagSet.Bool("n", false, "annotations only")
			payloadOnly := flagSet.Bool("doc", false, "orignal doc only")
			if err := flagSet.Parse(c.Args); err != nil {
				if err != flag.ErrHelp {
					c.Err(err)
				}
				return
			}
			argRest := flagSet.Args()
			if len(argRest) == 0 {
				c.Err(errors.New("missing source file"))
				return
			}

			srcName := argRest[0]

			node, err := ctx.api.Filetree.NodeByPath(srcName, ctx.node)

			if err != nil || node.IsDirectory() {
				c.Err(errors.New("file doesn't exist"))
				return
			}

			c.Println(fmt.Sprintf("downloading: [%s]...", srcName))

			zipName := fmt.Sprintf("%s.zip", node.Name())
			err = ctx.api.FetchDocument(node.Document.ID, zipName)

			if err != nil {
				c.Err(errors.New(fmt.Sprintf("Failed to download file %s with %s", srcName, err.Error())))
				return
			}
			if *payloadOnly {
				err = export(zipName, node.Name())
				if err != nil {
					c.Err(errors.New(fmt.Sprintf("Failed to get paylout %s with %s", srcName, err.Error())))
				}
				return
			}

			pdfName := fmt.Sprintf("%s-annotations.pdf", node.Name())
			options := annotations.PdfGeneratorOptions{AddPageNumbers: *addPageNumbers, AllPages: *allPages, AnnotationsOnly: *annotationsOnly}
			generator := annotations.CreatePdfGenerator(zipName, pdfName, options)
			err = generator.Generate()

			if err != nil {
				c.Err(errors.New(fmt.Sprintf("Failed to generate annotations for %s with %s", srcName, err.Error())))
				return
			}

			c.Printf("Annotations generated in: %s\n", pdfName)
		},
	}
}
