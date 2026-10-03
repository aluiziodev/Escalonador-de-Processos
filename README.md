# Escalonador de Processos - SISTEMAS OPERACIONAIS 2026.2

Aplicação para simular algoritmos de escalonamento de tarefas. O projeto combina uma interface web, usada para configurar processos e visualizar a execução, com uma API REST em Go que realiza as simulações.

Projeto realizado para a disciplina de Sistemas Operacionais 2026.2 - Departamemto de Computação da UFC.

## Funcionalidades

- Seleção entre sete políticas de escalonamento.
- Cadastro de processos pela tabela ou pela entrada em texto.
- Configuração de quantum e envelhecimento (aging) quando aplicável.
- Visualização da execução em diagrama de Gantt ou em formato textual, com controles de reprodução e velocidade.
- Exibição de métricas por processo e médias de turnaround, espera e resposta, além do tempo total e das trocas de contexto.

## Algoritmos disponíveis

| Identificador | Algoritmo | Características |
| --- | --- | --- |
| `fcfs` | First Come, First Served | Não preemptivo |
| `sjf` | Shortest Job First | Não preemptivo |
| `srtf` | Shortest Remaining Time First | Preemptivo |
| `prio-np` | Escalonamento por prioridade | Não preemptivo |
| `prio-p` | Escalonamento por prioridade | Preemptivo |
| `rr` | Round Robin | Usa quantum |
| `rr-prio-aging` | Round Robin com prioridade e aging | Usa quantum, prioridade e aging |

Nos algoritmos de prioridade, por padrão, um número menor representa uma prioridade maior, mas pode ser alterado dentro do BackEnd, em `backend/internal/sheduler/engine.go`:

```go
// LowerNumberIsHigherPriority indica que a prioridade menor corresponde a maior importância.
const LowerNumberIsHigherPriority = false
```

## Tecnologias

- **Backend:** Go e `net/http` da biblioteca padrão; não há dependências externas no módulo Go.
- **Frontend:** HTML, CSS e JavaScript sem framework ou etapa de build.
- **Fonte tipográfica:** Fira Sans e Roboto Condensed, carregadas do Google Fonts quando há conexão com a internet.

## Estrutura do projeto

```text
.
├── backend/                 # API e núcleo do escalonador em Go
│   ├── cmd/api/             # Inicialização do servidor HTTP
│   ├── internal/            # Rotas, controllers, serviços, DTOs e algoritmos
│   ├── go.mod
│   └── README.md            # Documentação detalhada do backend
├── docs/                    # Documentação complementar do projeto
├── public/                  # Interface web
│   ├── scheduler.html
│   ├── script.js
│   └── style.css
└── README.md
```

## Requisitos

- Go na versão indicada em `backend/go.mod` (ou compatível com ela).
- Python 3 para servir a interface localmente, ou outro servidor HTTP estático.
- Navegador moderno.

Não é necessário instalar pacotes npm.

## Como executar

A interface depende da API. Inicie cada parte em um terminal separado, a partir da raiz do repositório.

### 1. Iniciar o backend

```powershell
Set-Location backend
go mod download
 go run ./cmd/api
```

A API ficará disponível em `http://localhost:8080`.

### 2. Servir o frontend

Em outro terminal, na raiz do repositório:

```powershell
py -m http.server 5500 --directory public
```

Se o comando `py` não estiver disponível, use `python` no lugar. Abra então `http://localhost:5500/scheduler.html` no navegador. Também é possível usar qualquer servidor estático local apontado para a pasta `public`.

O frontend usa `http://localhost:8080` como endereço da API. Se a API estiver em outro endereço, ajuste `API_BASE` em [public/script.js](public/script.js).


## API

A API fica disponível em `http://localhost:8080` e habilita CORS para integração com a interface.

- `GET /algorithm`: lista os algoritmos disponíveis e seus metadados.
- `POST /simulate`: executa uma simulação e retorna processos, métricas, intervalos de CPU e timeline.

A documentação do contrato da API, dos parâmetros, da arquitetura interna e dos detalhes de implementação está no [README do backend](backend/README.md).

## Documentação complementar

Consulte a pasta [docs](docs/) para materiais adicionais sobre a implementação do escalonador e equipe.
