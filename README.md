# xva2qcow2

**xva2qcow2** é uma aplicação em Go que automatiza a conversão de arquivos XVA para QCOW2.

## Funcionalidades

- **Extração do XVA:** Extrai o arquivo tar para um diretório temporário.
- **Junção dos blocos:** Une arquivos numéricos (ex.: 00000000, 00000001, etc.) para gerar um arquivo raw.
- **Conversão para QCOW2:** Converte o arquivo raw usando o qemu-img (com exibição de progresso).
- **Limpeza:** Remove arquivos temporários e diretórios de extração após a conversão.

## Requisitos

- [Go](https://golang.org/)
- [qemu-img](https://www.qemu.org/)

## Instalação

Clone o repositório e compile:

```bash
git clone https://github.com/raotelecom/xva2qcow2.git
cd xva2qcow2
go build -o xva2qcow2 xva2qcow2.go
```

## Exemplo de uso

```bash
./xva2qcow2 -x <arquivo.xva> -o <saida.qcow2> [-r <refDir ou prefixo>]
```

## Contato
Ricardo Oliveira
ricardo@raotelecom.com.br