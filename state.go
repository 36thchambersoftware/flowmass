package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"sync"
)

// State tracks mint counter and processed deposits.
type State struct {
	mu                sync.Mutex
	filePath          string
	NextMintCounter   int             `json:"next_mint_counter"`
	ProcessedDeposits []string        `json:"processed_deposits"`
	PendingDeposits   map[string]int  `json:"pending_deposits"`
	processedSet      map[string]bool // in-memory cache
}

// LoadState loads state from file or initializes new.
func LoadState(filePath string) (*State, error) {
	state := &State{
		filePath:          filePath,
		NextMintCounter:   1,
		ProcessedDeposits: []string{},
		PendingDeposits:   make(map[string]int),
		processedSet:      make(map[string]bool),
	}

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist; save initial state
			if err := state.Save(); err != nil {
				return nil, err
			}
			log.Printf("[state] initialized new state file: %s", filePath)
			return state, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, state); err != nil {
		return nil, err
	}

	// Rebuild in-memory set
	for _, tx := range state.ProcessedDeposits {
		state.processedSet[tx] = true
	}
	if state.PendingDeposits == nil {
		state.PendingDeposits = make(map[string]int)
	}

	log.Printf("[state] loaded state: next_mint=%d, processed=%d deposits", state.NextMintCounter, len(state.ProcessedDeposits))
	return state, nil
}

// IsProcessed checks if a deposit tx has been processed.
func (s *State) IsProcessed(txHash string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.processedSet[txHash]
}

// MarkProcessed marks a deposit as processed.
func (s *State) MarkProcessed(txHash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.processedSet[txHash] {
		s.processedSet[txHash] = true
		s.ProcessedDeposits = append(s.ProcessedDeposits, txHash)
	}
}

// ReservePendingMint reserves the next mint id for a deposit and persists the state.
// Returns the reserved id. The reservation is recorded as depositTx -> id
// so that restarts won't reuse the id.
func (s *State) ReservePendingMint(depositTx string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.PendingDeposits == nil {
		s.PendingDeposits = make(map[string]int)
	}
	// If there's already a reservation for this deposit, return it (idempotent)
	if id, ok := s.PendingDeposits[depositTx]; ok {
		return id, nil
	}

	id := s.NextMintCounter
	s.NextMintCounter++
	s.PendingDeposits[depositTx] = id

	data, err := json.MarshalIndent(map[string]interface{}{
		"next_mint_counter":  s.NextMintCounter,
		"processed_deposits": s.ProcessedDeposits,
		"pending_deposits":   s.PendingDeposits,
	}, "", "  ")
	if err != nil {
		return 0, err
	}

	if err := ioutil.WriteFile(s.filePath, data, 0o600); err != nil {
		return 0, err
	}

	return id, nil
}

// ClearPending removes a pending reservation for a deposit and persists state.
func (s *State) ClearPending(depositTx string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.PendingDeposits != nil {
		delete(s.PendingDeposits, depositTx)
	}

	data, err := json.MarshalIndent(map[string]interface{}{
		"next_mint_counter":  s.NextMintCounter,
		"processed_deposits": s.ProcessedDeposits,
		"pending_deposits":   s.PendingDeposits,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(s.filePath, data, 0o600); err != nil {
		return err
	}
	return nil
}

// NextMintID returns and increments the mint counter.
func (s *State) NextMintID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.NextMintCounter
	s.NextMintCounter++
	return id
}

// ReserveNextMintID increments the counter and persists the state immediately.
// This ensures reserved token names are not reused even if the process crashes later.
func (s *State) ReserveNextMintID() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.NextMintCounter
	s.NextMintCounter++

	data, err := json.MarshalIndent(map[string]interface{}{
		"next_mint_counter":  s.NextMintCounter,
		"processed_deposits": s.ProcessedDeposits,
	}, "", "  ")
	if err != nil {
		return 0, err
	}

	if err := ioutil.WriteFile(s.filePath, data, 0o600); err != nil {
		return 0, err
	}

	return id, nil
}

// Save persists state to file.
func (s *State) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(map[string]interface{}{
		"next_mint_counter":  s.NextMintCounter,
		"processed_deposits": s.ProcessedDeposits,
	}, "", "  ")
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(s.filePath, data, 0o600); err != nil {
		return err
	}

	return nil
}
