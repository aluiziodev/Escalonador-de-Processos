package scheduler

import (
	"fmt"
	"math/rand"
	"time"
)

// LowerNumberIsHigherPriority indica que a prioridade menor corresponde a maior importância.
const LowerNumberIsHigherPriority = true

// Engine é a estrutura que mantém o estado da simulação
type Engine struct {
	Processes []*Process
	Ready     []*Process // Chegaram e nao terminaram
	Last      *Process   // O Atual (se ainda estiver em exec) ou ultimo processo que foi executado
	Slice     int        // ticks consumidos do quantum
	T         int        // tick atual
	Quantum   int
	Aging     int
	Rng       *rand.Rand
}

// Retorna o processo que está sendo executado no momento e se ainda resta tempo de execução, ou nil caso não haja nenhum
func (e *Engine) Current() *Process {
	if e.Last != nil && e.Last.remaining > 0 {
		return e.Last
	}
	return nil
}

// Adiciona os processos que chegaram no tick atual à fila de prontos
func (e *Engine) Push() {
	for _, p := range e.Processes {
		if p.Arrival == e.T {
			e.Ready = append(e.Ready, p)
		}
	}
}

// Remove o processo da fila de prontos
func (e *Engine) removeFromReady(p *Process) {
	for i, process := range e.Ready {
		if process == p {
			e.Ready = append(e.Ready[:i], e.Ready[i+1:]...)
			break
		}
	}
}

// Retorna o processo com a menor chave, de acordo com a função key fornecida.
// Em caso de empate, aplica os seguintes critérios de desempate:
// (i) Se algum dos processos empatados já está com o processador, ele é escolhido.
// (ii) Se ainda houver empate, o processo com menor tempo restante é escolhido.
// (iii) Se ainda houver empate, um dos processos empatados é escolhido aleatoriamente.
func (e *Engine) Best(key func(p *Process) int) *Process {
	var candidates []*Process
	bestKey := -1
	for _, p := range e.Ready {
		k := key(p)
		if bestKey == -1 || k < bestKey {
			bestKey = k
			candidates = []*Process{p}
		} else if k == bestKey {
			candidates = append(candidates, p)
		}
	}

	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	// (i) Ja esta com o processador

	for _, p := range candidates {
		if p == e.Last {
			return p
		}
	}

	// (ii) Menor tempo restante
	minRemaining := candidates[0].remaining
	for _, p := range candidates {
		if p.remaining < minRemaining {
			minRemaining = p.remaining
		}
	}
	var remainingCandidates []*Process
	for _, p := range candidates {
		if p.remaining == minRemaining {
			remainingCandidates = append(remainingCandidates, p)
		}
	}

	if len(remainingCandidates) == 1 {
		return remainingCandidates[0]
	}

	// (iii) Aleatorio
	return remainingCandidates[e.Rng.Intn(len(remainingCandidates))]

}

// Cria uma linha do tempo com o estado de cada processo no tick atual
func (e *Engine) row(running *Process) TimeLineRow {
	states := map[string]string{}
	for _, p := range e.Processes {
		if p.Arrival <= e.T && p.remaining > 0 {
			if p == running {
				states[p.Name] = "running"
			} else {
				states[p.Name] = "ready"
			}
		}
	}
	return TimeLineRow{From: e.T, To: e.T + 1, States: states}
}

// Run executa a simulação de escalonamento para um conjunto de processos e retorna as métricas finais.
func Run(s Simulate) (*Result, error) {
	meta, ok := Algorithms[s.Algorithm]
	if !ok {
		return nil, fmt.Errorf("algoritmo desconhecido: %s", s.Algorithm)
	}

	if len(s.Processes) == 0 {
		return nil, fmt.Errorf("nenhum processo fornecido")
	}

	if meta.UsesQuantum && s.Quantum < 1 {
		return nil, fmt.Errorf("quantum deve ser maior que 0")
	}

	if meta.UsesAging && s.Aging < 1 {
		return nil, fmt.Errorf("aging deve ser maior que 0")
	}

	// Gera uma fonte aleatória para desempates em casos de igualdade entre processos.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Inicializa o motor da simulação com os parâmetros do algoritmo selecionado.
	e := &Engine{Quantum: s.Quantum, Aging: s.Aging, Rng: rng}
	for i, p := range s.Processes {
		if p.Arrival < 0 || p.Burst < 1 {
			return nil, fmt.Errorf("processo %d tem valores inválidos: arrival=%d, burst=%d", i, p.Arrival, p.Burst)
		}

		// Ajusta a prioridade para o critério interno da simulação (menor valor = maior prioridade).
		base := p.Priority
		if !LowerNumberIsHigherPriority {
			base = -base
		}

		e.Processes = append(e.Processes, &Process{
			Id:        i + 1,
			Name:      p.Name,
			Arrival:   p.Arrival,
			Burst:     p.Burst,
			Priority:  p.Priority,
			remaining: p.Burst,
			start:     -1,
			finish:    -1,
			base:      base,
			key:       base,
		})
	}

	// Seleciona a política de escalonamento correta para a execução.
	alg := newAlgorithm(s.Algorithm)
	res := &Result{Algorithm: s.Algorithm, Intervals: []Interval{}, Timeline: []TimeLineRow{}}

	// Laço principal: cada iteração representa um tick de processamento da CPU.
	done := 0
	e.Push()
	for done < len(e.Processes) {
		run := alg.pick(e)
		res.Timeline = append(res.Timeline, e.row(run))

		if run == nil { // CPU ociosa: não há processo pronto para executar neste instante.
			e.Last, e.Slice = nil, 0
			e.T++
			e.Push()
			continue
		}

		// Quando o processo em execução muda, contabiliza a troca de contexto.
		if run != e.Last {
			if e.Last != nil {
				res.ContextSwitches++
			}
			e.Slice = 0
		}

		if run.start < 0 {
			run.start = e.T
		}

		// Agrupa execuções contínuas do mesmo processo em um único intervalo de tempo.
		if n := len(res.Intervals); n > 0 && res.Intervals[n-1].Id == run.Id &&
			res.Intervals[n-1].Finish == e.T {
			res.Intervals[n-1].Finish++
		} else {
			res.Intervals = append(res.Intervals, Interval{
				Id:     run.Id,
				Start:  e.T,
				Finish: e.T + 1,
			})
		}

		// Executa um tick do processo atual e atualiza o estado do quantum e do tempo global.
		run.remaining--
		e.Slice++
		e.T++

		fineshed := run.remaining == 0
		if fineshed {
			run.finish = e.T
			e.removeFromReady(run)
			done++
		}
		e.Last = run

		// Se o processo terminou ou o quantum expirou, a próxima escolha deve considerar reinício do slice.
		sliceEnded := fineshed || (e.Quantum > 0 && e.Slice >= e.Quantum)
		if sliceEnded {
			e.Slice = 0
		}

		// Novos processos só entram antes da possível volta do processo preemptado à fila.
		e.Push()

		alg.after(e, run, sliceEnded)
	}

	// Calcula tempo de turnaround, waiting e response para cada processo.

	var sumT, sumW, sumR float64

	for _, p := range e.Processes {
		turn := p.finish - p.Arrival
		wait := turn - p.Burst
		resp := p.start - p.Arrival

		res.Processes = append(res.Processes, ProcessResult{
			Id:         p.Id,
			Name:       p.Name,
			Arrival:    p.Arrival,
			Burst:      p.Burst,
			Priority:   p.Priority,
			Start:      p.start,
			Finish:     p.finish,
			Turnaround: turn,
			Waiting:    wait,
			Response:   resp,
		})

		if p.finish > res.TotalTime {
			res.TotalTime = p.finish
		}

		sumT += float64(turn)
		sumW += float64(wait)
		sumR += float64(resp)

	}

	n := float64(len(e.Processes))
	res.Averages = Averages{Turnaround: sumT / n, Waiting: sumW / n, Response: sumR / n}

	return res, nil
}
