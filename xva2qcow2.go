package main

import (
	"archive/tar"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const blockSize = 1024 * 1024 // 1 MB

// extractXVA extrai o arquivo XVA (tar) para o diretório destDir.
func extractXVA(xvaPath, destDir string) error {
	f, err := os.Open(xvaPath)
	if err != nil {
		return err
	}
	defer f.Close()

	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		targetPath := filepath.Join(destDir, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			outFile, err := os.Create(targetPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	return nil
}

// joinBlocks concatena os arquivos numéricos do diretório refDir e gera o arquivo raw.
func joinBlocks(refDir, rawOutput string) error {
	files, err := ioutil.ReadDir(refDir)
	if err != nil {
		return err
	}
	maxNum := -1
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		n, err := strconv.Atoi(file.Name())
		if err == nil && n > maxNum {
			maxNum = n
		}
	}
	if maxNum < 0 {
		return fmt.Errorf("nenhum arquivo numérico encontrado em %s", refDir)
	}
	totalBlocks := maxNum + 1

	out, err := os.Create(rawOutput)
	if err != nil {
		return err
	}
	defer out.Close()

	notificationInterval := 1024
	fmt.Printf("last file: %d\n", totalBlocks)
	fmt.Printf("disk image size: %.2f GB\n", float64(totalBlocks)/1024.0)
	fmt.Println("Unindo blocos:")

	for i := 0; i < totalBlocks; i++ {
		filename := fmt.Sprintf("%08d", i)
		filePath := filepath.Join(refDir, filename)
		if _, err := os.Stat(filePath); err == nil {
			in, err := os.Open(filePath)
			if err != nil {
				return err
			}
			buf := make([]byte, blockSize)
			for {
				n, err := in.Read(buf)
				if n > 0 {
					if _, werr := out.Write(buf[:n]); werr != nil {
						in.Close()
						return werr
					}
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					in.Close()
					return err
				}
			}
			in.Close()
		} else {
			// Se o arquivo não existir, avança o ponteiro do arquivo.
			if _, err := out.Seek(blockSize, io.SeekCurrent); err != nil {
				return err
			}
		}
		if (i+1)%notificationInterval == 0 {
			fmt.Printf("Processado %d GB...\n", (i+1)/notificationInterval)
		}
	}
	fmt.Println("Arquivo raw criado com sucesso!")
	return nil
}

// autoDetectRefDir procura por um subdiretório cujo nome comece com o prefixo fornecido.
func autoDetectRefDir(baseDir, prefix string) string {
	entries, err := ioutil.ReadDir(baseDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			return filepath.Join(baseDir, entry.Name())
		}
	}
	return ""
}

func main() {
	xvaPath := flag.String("x", "", "Caminho para o arquivo XVA de entrada")
	outputPath := flag.String("o", "", "Caminho para o arquivo qcow2 de saída")
	refDirName := flag.String("r", "", "Nome (ou prefixo) do diretório com os blocos extraídos (padrão: auto-detecta diretório que comece com 'Ref:')")
	flag.Parse()

	if *xvaPath == "" || *outputPath == "" {
		fmt.Println("Uso: xva2qcow2 -x <arquivo.xva> -o <saida.qcow2> [-r <refDir ou prefixo>]")
		os.Exit(1)
	}

	// Verifica se a extensão de saída é .qcow2, caso contrário substitui a existente
	if strings.ToLower(filepath.Ext(*outputPath)) != ".qcow2" {
		base := strings.TrimSuffix(*outputPath, filepath.Ext(*outputPath))
		*outputPath = base + ".qcow2"
	}

	// Define o diretório onde o XVA está localizado e cria uma pasta de extração nesse mesmo local.
	xvaAbs, err := filepath.Abs(*xvaPath)
	if err != nil {
		fmt.Println("Erro ao obter caminho absoluto do XVA:", err)
		os.Exit(1)
	}
	xvaDir := filepath.Dir(xvaAbs)
	baseName := filepath.Base(*xvaPath)
	extExtractionDir := filepath.Join(xvaDir, baseName+"_extracted")

	// Remove o diretório de extração se já existir para garantir uma extração limpa.
	os.RemoveAll(extExtractionDir)
	if err := os.MkdirAll(extExtractionDir, 0755); err != nil {
		fmt.Println("Erro ao criar diretório de extração:", err)
		os.Exit(1)
	}

	fmt.Println("Extraindo XVA para:", extExtractionDir)
	if err := extractXVA(*xvaPath, extExtractionDir); err != nil {
		fmt.Println("Erro na extração do XVA:", err)
		os.Exit(1)
	}

	// Determina o diretório com os blocos.
	var refDir string
	if *refDirName != "" {
		prefix := *refDirName
		if !strings.Contains(prefix, ":") {
			prefix = "Ref:"
		}
		refDir = autoDetectRefDir(extExtractionDir, prefix)
	} else {
		// Se não for fornecido, auto-detecta o diretório que comece com "Ref:".
		refDir = autoDetectRefDir(extExtractionDir, "Ref:")
		if refDir == "" {
			refDir = extExtractionDir
		}
	}

	if refDir == "" {
		fmt.Println("Erro: não foi possível detectar o diretório dos blocos.")
		os.Exit(1)
	}

	fmt.Println("Unindo blocos a partir de:", refDir)

	// Cria o arquivo raw temporário na mesma pasta do XVA
	rawTemp := filepath.Join(xvaDir, baseName+".raw")
	if err := joinBlocks(refDir, rawTemp); err != nil {
		fmt.Println("Erro ao unir blocos:", err)
		os.Exit(1)
	}

	// Converte o arquivo raw para qcow2 chamando o qemu-img.
	fmt.Println("Convertendo raw para qcow2 usando qemu-img...")
	cmd := exec.Command("qemu-img", "convert", "-p", "-O", "qcow2", rawTemp, *outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("Erro ao converter com qemu-img:", err)
		os.Exit(1)
	}

	// Remove o arquivo raw temporário.
	if err := os.Remove(rawTemp); err != nil {
		fmt.Println("Aviso: não foi possível remover o arquivo raw temporário:", err)
	}

	// Remove o diretório de extração.
	if err := os.RemoveAll(extExtractionDir); err != nil {
		fmt.Println("Aviso: não foi possível remover o diretório de extração:", err)
	}

	fmt.Println("Arquivo qcow2 gerado com sucesso:", *outputPath)
}
