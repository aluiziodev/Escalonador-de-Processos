# Backend - Escalonador de Processos
 
## Visão geral
 
Este backend foi desenvolvido em Go para simular algoritmos de escalonamento de processos e expor os resultados por meio de uma API REST. Ele recebe uma lista de processos, aplica o algoritmo solicitado e retorna métricas como tempo total, tempo de espera, turnaround, tempo de resposta, número de trocas de contexto e a linha do tempo da execução.
 
A aplicação é voltada para integração com a interface web do projeto, permitindo que a página front-end envie os dados de entrada e receba o resultado da simulação em formato JSON.
 
## Sumário
 
1. [Tecnologias](#tecnologias)
2. [Estrutura do backend](#estrutura-do-backend)
3. [Design e arquitetura](#design-e-arquitetura)
4. [Estrutura de controle de cada processo (PCB)](#estrutura-de-controle-de-cada-processo-pcb)
5. [Classes (tipos) e responsabilidades](#classes-tipos-e-responsabilidades)
6. [Estruturas de dados utilizadas](#estruturas-de-dados-utilizadas)
7. [Padrões de projeto](#padrões-de-projeto)
8. [Decisões de implementação do escalonador](#decisões-de-implementação-do-escalonador)
9. [Endpoints](#endpoints)
10. [Algoritmos suportados](#algoritmos-suportados)
11. [Tratamento de CORS](#tratamento-de-cors)
12. [Testes](#testes)
13. [Como executar](#como-executar)
14. [Observações de design](#observações-de-design)
## Tecnologias
 
- Go
- HTTP Server nativo da biblioteca padrão (`net/http`, sem frameworks externos)

## Estrutura do backend
 
```text
backend/
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── controllers/
│   │   └── scheduler.go
│   ├── dto/
│   │   ├── requestSimulate.go
│   │   ├── responseAlgorithms.go
│   │   └── responseSimulate.go
│   ├── response/
│   │   └── response.go
│   ├── router/
│   │   ├── router.go
│   │   └── routes.go
│   ├── scheduler/
│   │   ├── algorithms.go
│   │   ├── algorithms_test.go
│   │   ├── engine.go
│   │   ├── process.go
│   │   └── simulate.go
│   └── service/
│       └── scheduler.go
├── go.mod
└── README.md
```
 
## Design e arquitetura
 
O backend segue uma estrutura em camadas para separar responsabilidades:
 
1. `cmd/api/main.go`
   - Ponto de entrada da aplicação.
   - Inicializa o roteador HTTP e sobe o servidor na porta `8080` (com `ReadHeaderTimeout` de 5 segundos).
2. `internal/router`
   - Define as rotas HTTP (`routes`) e habilita CORS para permitir chamadas vindas do frontend.
   - Cada rota mapeia para uma função do controller correspondente.
3. `internal/controllers`
   - Recebe as requisições HTTP.
   - Decodifica o JSON enviado pelo cliente.
   - Chama o serviço apropriado e responde no formato JSON.
4. `internal/service`
   - Converte os DTOs de entrada em estruturas internas do scheduler.
   - Invoca a lógica de simulação e transforma os resultados em DTOs de resposta.
5. `internal/scheduler`
   - Contém o núcleo da lógica de escalonamento.
   - Implementa os algoritmos, a validação de parâmetros, o cálculo das métricas e a geração da linha do tempo.
   - Não depende de nenhum outro pacote do projeto (nem de HTTP, nem de DTOs), o que permite testá-lo isoladamente.
6. `internal/dto`
   - Define as estruturas de entrada e saída da API (com as tags `json` para serializar).
   - Mantém a comunicação entre camadas estável e organizada.
7. `internal/response`
   - Funções utilitárias para escrever respostas JSON (sucesso e erro) de forma padronizada.
### Fluxo de execução
 
```text
Frontend --> HTTP Request --> Router (CORS) --> Controller --> Service --> Scheduler (Run) --> Resultados --> Response JSON
```
 
Sentido das dependências entre pacotes (cada camada só conhece a de baixo):
 
```text
router ──► controllers ──► service ──► scheduler
                │             │
                └──► dto ◄────┘
                └──► response
```
 
## Estrutura de controle de cada processo (PCB)
 
Cada processo da simulação é representado pela struct `Process` (`internal/scheduler/process.go`), que funciona como o **bloco de controle de processo (PCB)** do simulador. Ela reúne tanto os dados informados pelo usuário quanto os campos internos usados para controlar a execução:
 
```go
type Process struct {
    Id        int    // identificador único (1, 2, 3... na ordem de entrada)
    Name      string // nome informado pelo usuário (ex.: "P1")
    Arrival   int    // instante de chegada
    Burst     int    // tempo total de CPU necessário
    Priority  int    // prioridade informada (menor número = maior prioridade)
    remaining int    // tempo de CPU que ainda falta executar
    start     int    // instante da primeira execução (-1 = ainda não executou)
    finish    int    // instante de término (-1 = ainda não terminou)
    base      int    // prioridade estática usada internamente
    key       int    // prioridade efetiva (muda com o aging)
}
```
 
| Campo | Visibilidade | Origem | Papel no controle |
|-------|--------------|--------|-------------------|
| `Id` | exportado | atribuído em `Run` (`índice + 1`) | Identificador único. É a chave usada para agrupar `Interval`s consecutivos do mesmo processo e para associar o intervalo ao nome do processo na resposta. |
| `Name` | exportado | requisição | Rótulo exibido ao usuário. Também é a chave do mapa de estados da timeline. |
| `Arrival` | exportado | requisição | Define o momento em que o processo entra na fila de prontos. |
| `Burst` | exportado | requisição | Tempo total de CPU. Usado como critério no SJF e no cálculo do waiting. |
| `Priority` | exportado | requisição | Prioridade original, devolvida na resposta. |
| `remaining` | interno | inicia igual a `Burst` | Decrementado a cada tick executado. O processo termina quando chega a `0`. Critério do SRTF e do desempate. |
| `start` | interno | inicia em `-1` | Gravado na primeira vez que o processo recebe a CPU. Base do tempo de resposta. |
| `finish` | interno | inicia em `-1` | Gravado quando `remaining` chega a `0`. Base do turnaround. |
| `base` | interno | `Priority` (ver abaixo) | Prioridade estática usada nos algoritmos `prio-np`, `prio-p` e como valor de reset no aging. |
| `key` | interno | inicia igual a `base` | Prioridade dinamica do `rr-prio-aging`: diminui (ganha prioridade) quando o processo espera. |
 
### Status do processo
 
O backend não guarda um campo `Status` dentro da struct `Process`. O status é derivado do estado da simulação a cada tick, em `Engine.row()`, e é devolvido na `timeline` da resposta:
 
| Status | Como é determinado | Como aparece na timeline |
|--------|--------------------|--------------------------|
| **novo / não chegou** | `Arrival > T` | ausente no mapa `states` |
| **ready** (pronto) | `Arrival <= T`, `remaining > 0` e não é o processo escolhido no tick | `"ready"` |
| **running** (executando) | é o processo escolhido pelo algoritmo no tick | `"running"` |
| **finished** (terminado) | `remaining == 0` (e `finish` preenchido) | ausente no mapa `states` |
 
Além disso, o pertencimento à fila `Engine.Ready` também é parte do controle: um processo está em `Ready` se já chegou e ainda não terminou.
 
### Estrutura de resultado por processo
 
Ao final da simulação, cada `Process` é convertido em um `ProcessResult` (`internal/scheduler/simulate.go`), que contém os dados originais mais as métricas calculadas:
 
```go
type ProcessResult struct {
    Id, Name                      // identificação
    Arrival, Burst, Priority      // dados de entrada
    Start, Finish                 // primeira execução e término
    Turnaround, Waiting, Response // métricas
}
```
 
## Classes e responsabilidades
 
Como Go não possui classes, os conceitos de orientação a objetos são implementados com `struct`'s, métodos e interfaces.
 
| Tipo | Arquivo | Responsabilidade |
|------|---------|------------------|
| `Process` | `scheduler/process.go` | PCB do processo. |
| `Simulate` | `scheduler/simulate.go` | Entrada do núcleo: algoritmo, `Quantum`, `Aging` e lista de `Process`. |
| `Result` | `scheduler/simulate.go` | Saída do núcleo: tempo total, trocas de contexto, processos, intervalos, timeline e médias. |
| `ProcessResult` | `scheduler/simulate.go` | Métricas finais de cada processo. |
| `Interval` | `scheduler/simulate.go` | Intervalo contínuo em que um processo ocupou a CPU (base do diagrama). |
| `TimeLineRow` | `scheduler/simulate.go` | Estado de todos os processos em um tick (`From`, `To`, `States`). |
| `Averages` | `scheduler/simulate.go` | Médias de turnaround, waiting e response. |
| `Engine` | `scheduler/engine.go` | Estado da simulação (relógio, fila de prontos, último processo, quantum consumido) e operações comuns a todos os algoritmos (`Push`, `Current`, `Best`, `removeFromReady`, `row`). |
| `MetaData` | `scheduler/algorithms.go` | Descreve um algoritmo: se é preemptivo e se usa quantum, aging e prioridade. |
| `algorithm` (interface) | `scheduler/algorithms.go` | Contrato de uma política de escalonamento: `pick` e `after`. |
| `minorKey` | `scheduler/algorithms.go` | Implementação genérica para FCFS, SJF, SRTF, Priority NP e Priority P. |
| `roundRobin` | `scheduler/algorithms.go` | Implementação do Round Robin. |
| `rrAging` | `scheduler/algorithms.go` | Implementação do Round Robin com prioridade e aging. |
| `RequestSimulate` / `RequestProcess` | `dto/requestSimulate.go` | Corpo da requisição `POST /simulate`. |
| `ResponseSimulate`, `ResponseProcess`, `ResponseIntervals`, `ResponseTimeline`, `ResponseAverages` | `dto/responseSimulate.go` | Corpo da resposta `POST /simulate`. |
| `ResponseAlgorithms` / `ResponseAlgorithm` | `dto/responseAlgorithms.go` | Corpo da resposta `GET /algorithm`. |
| `route` | `router/routes.go` | Par URI + método HTTP + função tratadora. |
 
 
## Estruturas de dados utilizadas
 
| Estrutura | Onde | Por que foi escolhida |
|-----------|------|-----------------------|
| `[]*Process` (slice de ponteiros) - `Engine.Processes` | `engine.go` | Mantém todos os processos na ordem de entrada. Usar ponteiros permite que a mesma instância seja compartilhada entre `Processes`, `Ready` e `Last`, de modo que uma alteração (ex: `remaining--`) seja vista por todos. |
| `[]*Process` - `Engine.Ready` (fila de prontos) | `engine.go` | Contém os processos que já chegaram e não terminaram. No Round Robin funciona como fila: o processo no índice `0` é o próximo e o processo preemptado é reinserido no final com `append`. Nos demais algoritmos é percorrida linearmente para achar o melhor candidato.| `map[string]MetaData` - `Algorithms` | `algorithms.go` | Registro dos algoritmos com busca O(1) por identificador e validação de algoritmo inexistente. |
| `[]string` - `AlgorithmOrder` | `algorithms.go` | Garante ordem determinística na listagem de `GET /algorithm` (mapas em Go não têm ordem). |
| `[]Interval` | `simulate.go` | Lista de intervalos de execução; ticks consecutivos do mesmo processo são fundidos em um único intervalo. |
| `[]TimeLineRow` com `map[string]string` | `simulate.go` | Uma linha por tick, com o mapa `nome do processo -> estado`. |
| `func(p *Process) int` (função como chave) | `algorithms.go` | Passada para `Engine.Best` para definir o critério de ordenação sem duplicar código. |
| Tabela `[]route` | `router/routes.go` | Lista declarativa de endpoints, percorrida para registrar as rotas no `ServeMux`. |
 
Observação: a fila de prontos é um slice e não um heap/fila de prioridade de propósito. O número de processos de uma simulação didática é pequeno, e a varredura linear facilita aplicar os critérios de desempate e o aging (que altera a chave de vários processos ao mesmo tempo).
 
## Padrões de projeto
 
| Padrão | Onde aparece | Como é aplicado |
|--------|--------------|-----------------|
| **Strategy** | interface `algorithm` + `minorKey`, `roundRobin`, `rrAging` | Cada política de escalonamento é uma estratégia intercambiável. O laço de `Run` só conhece a interface (`pick` e `after`) e não precisa saber qual algoritmo está em uso. Para criar um novo algoritmo basta implementar a interface. |
| **Factory (Simple Factory)** | `newAlgorithm(id string) algorithm` | Centraliza a criação da estratégia a partir do identificador recebido na requisição. |
| **Registry / Lookup table** | `Algorithms` (`map[string]MetaData`) e `AlgorithmOrder` | Catálogo único de algoritmos e suas características. É usado tanto para validar a entrada quanto para alimentar o endpoint `GET /algorithm`. |
| **Template Method (estrutural)** | `Run` | `Run` define o esqueleto fixo de cada tick (escolher, registrar timeline, executar, atualizar quantum, admitir chegadas). Os pontos variáveis são delegados a `pick` e `after`. |
| **Layered Architecture** | `controllers → service → scheduler` | Separação entre transporte HTTP, conversão/orquestração e regra de negócio. |
| **DTO (Data Transfer Object)** | pacote `dto` | Desacopla o contrato JSON da API das structs internas. O núcleo não conhece tags `json`, e o contrato pode mudar sem mexer no algoritmo. |
| **Decorator / Middleware** | `withCORS` | Envolve o `http.Handler` para adicionar cabeçalhos CORS e tratar `OPTIONS` sem alterar os controllers. |
| **Table-driven routing** | `routes` em `router/routes.go` | As rotas são dados, não código repetido. |
 
## Decisões de implementação do escalonador
 
### Simulação discreta por ticks
 
O motor avança o relógio de 1 em 1 unidade de tempo. A cada tick ele executa o seguinte ciclo (`Run` em `engine.go`):
 
1. `alg.pick(e)` escolhe o processo que vai usar a CPU.
2. Registra uma linha na timeline com o estado de cada processo.
3. Se nenhum processo estiver pronto, a CPU fica ociosa e o relógio avança.
4. Se o processo escolhido é diferente do `Last`, contabiliza a troca de contexto e zera o `Slice`.
5. Registra `start` na primeira execução e estende (ou cria) o `Interval` atual.
6. Executa 1 tick: `remaining--`, `Slice++`, `T++`.
7. Se `remaining == 0`, registra `finish` e remove o processo da fila de prontos.
8. Calcula `sliceEnded` (processo terminou **ou** quantum esgotado) e zera `Slice` nesse caso.
9. `Push()` admite os processos que chegam no novo instante.
10. `alg.after(...)` permite que o algoritmo atualize seu estado (reinserir na fila, aplicar aging).
A ordem dos passos 9 e 10 é intencional: **novos processos entram na fila antes do processo preemptado voltar ao final dela**. Esse é o comportamento convencional do Round Robin quando uma chegada coincide com o fim de um quantum.
 
### Preempção
 
- **Não preemptivos** (`fcfs`, `sjf`, `prio-np`): o método `pick` mantém o processo atual (`Current()`) enquanto ele não terminar.
- **Preemptivos** (`srtf`, `prio-p`): `pick` reavalia a cada tick e pode trocar de processo assim que um candidato melhor aparecer.
- **Por quantum** (`rr`, `rr-prio-aging`): o processo mantém a CPU enquanto `Slice > 0`. Ao esgotar o quantum, um novo processo é escolhido.
### Critério de cada algoritmo (`minorKey`)
 
`minorKey` escolhe sempre o processo com **menor chave**:
 
| Algoritmo | Chave |
|-----------|-------|
| `fcfs` | `Arrival` |
| `sjf` | `Burst` |
| `srtf` | `remaining` |
| `prio-np`, `prio-p` | `base` (prioridade) |
 
### Convenção de prioridade
 
A constante `LowerNumberIsHigherPriority = true` define que **menor número significa maior prioridade**. Se ela for alterada para `false`, `Run` inverte o sinal ao calcular `base`, sem alterar nenhum algoritmo.
 
### Critérios de desempate (`Engine.Best`)
 
Quando mais de um processo empata na chave, aplica-se, em ordem:
 
1. Se um dos empatados **já está com a CPU**, ele é mantido (evita trocas de contexto desnecessárias).
2. Senão, vence o de **menor tempo restante**.
3. Se ainda houver empate, a escolha é **aleatória** (`math/rand`, semente gerada a cada simulação).
Por conta do item 3, simulações com empates totais podem produzir resultados diferentes entre execuções.
 
### Round Robin
 
Usa `Engine.Ready` como fila FIFO. Chegadas de ticks diferentes mantêm sua ordem; processos que chegam no mesmo tick são ordenados pelos desempates do `Best` (processo atual, menor tempo restante e escolha aleatória em caso de novo empate). Quando o quantum termina e o processo ainda tem `remaining > 0`, ele é removido e reinserido no final da fila (`after`).
 
### Round Robin com prioridade e aging (`rr-prio-aging`)
 
- Cada processo começa com `key = base` (a prioridade informada).
- A escolha é feita pela menor `key`, respeitando o quantum.
- Ao final de cada fatia (`sliceEnded`):
  - o processo que acabou de executar tem `key` restaurada para `base`;
  - todos os demais processos prontos que já estavam esperando (`Arrival < T`) têm `key -= Aging`.
- Não há piso para a `key`: a prioridade efetiva pode ultrapassar a melhor prioridade estática, garantindo que um processo que espera por muito tempo acabe sendo escolhido (**evita inanição**).
- Esse algoritmo não usa a ordem da fila `Ready`, e sim a `key`. Empates seguem as regras de `Best`.
### Troca de contexto
 
Uma troca é contada quando o processo escolhido é diferente de `Last` e `Last` não é `nil`. Logo:
 
- a troca após um processo terminar **é contada**;
- a primeira execução e o retorno de um período de CPU ociosa **não são contados** (nesses casos `Last` é `nil`);
- o custo da troca é considerado zero (não consome ticks).
### Métricas
 
Para cada processo:
 
```text
turnaround = finish - arrival
waiting    = turnaround - burst
response   = start - arrival
```
 
Além disso:
 
- `total_time` é o maior valor de `finish` entre os processos.
- `averages` é a média aritmética de cada métrica sobre todos os processos.
### Validações
 
Feitas no início de `Run`, antes de qualquer simulação:
 
- o algoritmo precisa existir em `Algorithms`;
- a lista de processos não pode ser vazia;
- `quantum >= 1` para algoritmos que usam quantum;
- `aging >= 1` para algoritmos que usam aging;
- cada processo precisa ter `arrival >= 0` e `burst >= 1`.
### Complexidade
 
Cada tick faz uma varredura linear sobre os processos (`Push`, `row`, `Best`). O custo total é aproximadamente **O(T × n)**, em que `T` é o tempo total simulado e `n` é o número de processos.
 
## Endpoints
 
A API roda em:
 
```text
http://localhost:8080
```
 
### 1) POST /simulate
 
Executa a simulação de escalonamento com os processos informados.
 
#### Requisição
 
```json
{
  "algorithm": "rr",
  "quantum": 2,
  "aging": 1,
  "processes": [
    { "name": "P1", "arrival": 0, "burst": 5, "priority": 2 },
    { "name": "P2", "arrival": 1, "burst": 3, "priority": 1 },
    { "name": "P3", "arrival": 2, "burst": 2, "priority": 3 }
  ]
}
```
 
#### Campos da requisição
 
- `algorithm`: algoritmo de escalonamento.
- `quantum`: quantum utilizado por algoritmos de time-sharing (ex.: `rr`).
- `aging`: valor de envelhecimento para algoritmos que utilizam prioridade com aging.
- `processes`: lista de processos.
Cada processo contém:
 
- `name`: nome do processo
- `arrival`: instante de chegada
- `burst`: tempo de CPU necessário
- `priority`: prioridade do processo 
O identificador (`Id`) não é enviado pelo cliente: ele é atribuído pelo backend seguindo a ordem da lista.
 
#### Resposta
 
Exemplo real gerado pela requisição acima (resposta abreviada nos campos `processes`, `intervals` e `timeline`):
 
```json
{
  "algorithm": "rr",
  "total_time": 10,
  "context_switches": 5,
  "processes": [
    {
      "name": "P1",
      "arrival": 0,
      "burst": 5,
      "priority": 2,
      "start": 0,
      "finish": 10,
      "waiting": 5,
      "turnaround": 10,
      "response": 0
    }
  ],
  "intervals": [
    { "process_name": "P1", "start": 0, "finish": 2 },
    { "process_name": "P2", "start": 2, "finish": 4 },
    { "process_name": "P3", "start": 4, "finish": 6 },
    { "process_name": "P1", "start": 6, "finish": 8 },
    { "process_name": "P2", "start": 8, "finish": 9 },
    { "process_name": "P1", "start": 9, "finish": 10 }
  ],
  "timeline": [
    { "from": 0, "to": 1, "states": { "P1": "running" } },
    { "from": 1, "to": 2, "states": { "P1": "running", "P2": "ready" } },
    { "from": 2, "to": 3, "states": { "P1": "ready", "P2": "running", "P3": "ready" } }
  ],
  "averages": {
    "turnaround": 7.33,
    "waiting": 4.0,
    "response": 1.0
  }
}
```
 
Campos da resposta:
 
- `total_time`: instante em que o último processo termina.
- `context_switches`: número de trocas de contexto.
- `processes`: dados originais e métricas de cada processo (`start`, `finish`, `waiting`, `turnaround`, `response`).
- `intervals`: intervalos contínuos de execução (útil para desenhar o diagrama de Gantt).
- `timeline`: estado de cada processo a cada tick. Os valores possíveis são `running` e `ready`; processos que ainda não chegaram ou que já terminaram não aparecem no mapa.
- `averages`: médias de turnaround, waiting e response.
#### Validações
 
A simulação exige que:
 
- o algoritmo exista;
- a lista de processos não esteja vazia;
- `quantum` seja maior que zero para algoritmos que usam quantum;
- `aging` seja maior que zero para algoritmos que usam aging;
- `arrival` seja maior ou igual a zero e `burst` maior ou igual a um.
Em caso de erro, a API retorna um JSON no seguinte formato:
 
```json
{
  "error": "mensagem detalhada do problema"
}
```
 
| Situação | Código HTTP |
|----------|-------------|
| JSON inválido no corpo da requisição | `400 Bad Request` |
| Falha de validação ou de simulação (`Run` retornou erro) | `500 Internal Server Error` |
| Sucesso | `200 OK` |
 
### 2) GET /algorithm
 
Retorna a lista de algoritmos suportados pelo backend, além de metadados sobre seu comportamento. A ordem é sempre a mesma (`AlgorithmOrder`).
 
#### Resposta exemplo
 
```json
{
  "algorithms": [
    {
      "algorithm": "fcfs",
      "preemptive": false,
      "uses_quantum": false,
      "uses_aging": false,
      "uses_priority": false
    },
    {
      "algorithm": "rr",
      "preemptive": true,
      "uses_quantum": true,
      "uses_aging": false,
      "uses_priority": false
    }
  ]
}
```
 
O frontend pode usar esses metadados para habilitar ou desabilitar os campos `quantum`, `aging` e `priority` conforme o algoritmo escolhido.
 
## Algoritmos suportados
 
A API oferece suporte aos seguintes algoritmos:
 
| Identificador | Algoritmo | Preemptivo | Quantum | Aging | Prioridade |
|---------------|-----------|:----------:|:-------:|:-----:|:----------:|
| `fcfs` | First Come, First Served | não | não | não | não |
| `sjf` | Shortest Job First | não | não | não | não |
| `srtf` | Shortest Remaining Time First | sim | não | não | não |
| `prio-np` | Priority sem preempção | não | não | não | sim |
| `prio-p` | Priority com preempção | sim | não | não | sim |
| `rr` | Round Robin | sim | sim | não | não |
| `rr-prio-aging` | Round Robin com prioridade e aging | sim | sim | sim | sim |
 
## Tratamento de CORS
 
O roteador habilita CORS com `Access-Control-Allow-Origin: *`, permitindo que o frontend, mesmo em outra origem, consiga consumir a API. Também trata requisições `OPTIONS` automaticamente, respondendo `204 No Content`.
 
## Testes
 
Os testes ficam em `internal/scheduler/algorithms_test.go` e cobrem cada um dos algoritmos (`fcfs`, `sjf`, `srtf`, `prio-np`, `prio-p`, `rr` e `rr-prio-aging`), validando os instantes de início e as métricas esperadas.
 
Para executá-los:
 
```bash
cd backend
go test ./...
```
 
## Como executar
 
Dentro da pasta `backend`:
 
```bash
cd backend
go mod download
go run ./cmd/api
```
 
Depois, a API ficará disponível em:
 
```text
http://localhost:8080
```
 
Exemplo de chamada:
 
```bash
curl -X POST http://localhost:8080/simulate \
  -H "Content-Type: application/json" \
  -d '{"algorithm":"fcfs","processes":[{"name":"P1","arrival":0,"burst":3,"priority":1}]}'
```
 
## Observações de design
 
- O backend foi construído para ser simples, didático e fácil de estender.
- A lógica de escalonamento fica isolada em `internal/scheduler`, permitindo que novas políticas sejam adicionadas sem alterar diretamente a camada HTTP.
- Para adicionar um novo algoritmo: (1) incluir uma entrada em `Algorithms` e em `AlgorithmOrder`, (2) implementar a interface `algorithm` (ou reaproveitar `minorKey` com outra função de chave) e (3) registrar o identificador em `newAlgorithm`.
- A separação entre DTOs, serviços e scheduler ajuda a manter o código organizado e reutilizável.
- A estrutura foi pensada para receber dados do frontend e retornar resultados em formato JSON padronizado.