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

// joinBlocks concatena os arquivos numéricos do diretório refDir e gera um arquivo raw.
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
			// Avança o ponteiro se o arquivo não existir
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

// autoDetectRefDirs retorna uma lista de subdiretórios dentro de baseDir cujo nome comece com o prefixo.
func autoDetectRefDirs(baseDir, prefix string) ([]string, error) {
	entries, err := ioutil.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			dirs = append(dirs, filepath.Join(baseDir, entry.Name()))
		}
	}
	return dirs, nil
}

func main() {
	xvaPath := flag.String("x", "", "Caminho para o arquivo XVA de entrada")
	outputPath := flag.String("o", "", "Prefixo para o arquivo qcow2 de saída")
	refDirPrefix := flag.String("r", "Ref:", "Prefixo dos diretórios com os blocos extraídos (padrão: 'Ref:')")
	flag.Parse()

	if *xvaPath == "" || *outputPath == "" {
		fmt.Println("Uso: xva2qcow2 -x <arquivo.xva> -o <saida> [-r <refDir prefixo>]")
		os.Exit(1)
	}

	// O argumento -o é tratado como prefixo; removemos a extensão, se houver.
	baseOutput := strings.TrimSuffix(*outputPath, ".qcow2")

	// Determina o diretório do XVA e cria o diretório de extração.
	xvaAbs, err := filepath.Abs(*xvaPath)
	if err != nil {
		fmt.Println("Erro ao obter caminho absoluto do XVA:", err)
		os.Exit(1)
	}
	xvaDir := filepath.Dir(xvaAbs)
	baseName := filepath.Base(*xvaPath)
	extExtractionDir := filepath.Join(xvaDir, baseName+"_extracted")

	// Remove e recria o diretório de extração para garantir uma extração limpa.
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

	// Detecta diretórios de blocos com prefixo "Ref:".
	diskDirs, err := autoDetectRefDirs(extExtractionDir, *refDirPrefix)
	if err != nil {
		fmt.Println("Erro ao detectar diretórios de blocos:", err)
		os.Exit(1)
	}
	// Se não encontrar nenhum, assume que há um único disco com blocos no próprio extExtractionDir.
	if len(diskDirs) == 0 {
		diskDirs = []string{extExtractionDir}
	}

	// Processa cada disco encontrado.
	for i, diskDir := range diskDirs {
		fmt.Printf("Processando disco %d a partir de: %s\n", i, diskDir)
		// Cria um arquivo raw temporário.
		rawTemp := filepath.Join(xvaDir, fmt.Sprintf("%s_disk_%d.raw", strings.TrimSuffix(baseName, filepath.Ext(baseName)), i))
		if err := joinBlocks(diskDir, rawTemp); err != nil {
			fmt.Printf("Erro ao unir blocos do disco %d: %v\n", i, err)
			os.Exit(1)
		}
		// Define o nome final do arquivo qcow2.
		var finalOutput string
		if len(diskDirs) > 1 {
			finalOutput = filepath.Join(xvaDir, fmt.Sprintf("%s-disk-%d.qcow2", baseOutput, i))
		} else {
			finalOutput = filepath.Join(xvaDir, baseOutput+".qcow2")
		}
		fmt.Printf("Convertendo raw do disco %d para qcow2: %s\n", i, finalOutput)
		// Converte o raw para qcow2 usando qemu-img com exibição de progresso.
		cmd := exec.Command("qemu-img", "convert", "-p", "-O", "qcow2", rawTemp, finalOutput)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Erro ao converter o disco %d: %v\n", i, err)
			os.Exit(1)
		}
		// Remove o arquivo raw temporário.
		if err := os.Remove(rawTemp); err != nil {
			fmt.Printf("Aviso: não foi possível remover o arquivo raw temporário do disco %d: %v\n", i, err)
		}
	}

	// Remove o diretório de extração.
	if err := os.RemoveAll(extExtractionDir); err != nil {
		fmt.Println("Aviso: não foi possível remover o diretório de extração:", err)
	}

	fmt.Println("Processo concluído com sucesso!")
}
