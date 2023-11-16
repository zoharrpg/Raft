//
// raft.go
// =======
// Write your code in this file
// We will use the original version of all other
// files for testing
//

package raft

//
// API
// ===
// This is an outline of the API that your raft implementation should
// expose.
//
// rf = NewPeer(...)
//   Create a new Raft server.
//
// rf.PutCommand(command interface{}) (index, term, isleader)
//   PutCommand agreement on a new log entry
//
// rf.GetState() (me, term, isLeader)
//   Ask a Raft peer for "me" (see line 58), its current term, and whether it thinks it
//   is a leader
//
// ApplyCommand
//   Each time a new entry is committed to the log, each Raft peer
//   should send an ApplyCommand to the service (e.g. tester) on the
//   same server, via the applyCh channel passed to NewPeer()
//

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/cmu440/rpc"
)

// Set to false to disable debug logs completely
// Make sure to set kEnableDebugLogs to false before submitting
const kEnableDebugLogs = false

// Set to true to log to stdout instead of file
const kLogToStdout = true

// Change this to output logs to a different directory
const kLogOutputDir = "./raftlogs/"

type State int

const (
	FOLLOWER State = iota
	CANDIDATE
	LEADER
)

// ApplyCommand
// ========
//
// As each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyCommand to the service (or
// tester) on the same server, via the applyCh passed to NewPeer()
type ApplyCommand struct {
	Index   int
	Command interface{}
}
type LogStruct struct {
	CommandInfo ApplyCommand
	Term        int
}

// Raft struct
// ===========
//
// A Go object implementing a single Raft peer
type Raft struct {
	mux   sync.Mutex       // Lock to protect shared access to this peer's state
	peers []*rpc.ClientEnd // RPC end points of all peers
	me    int              // this peer's index into peers[]
	// You are expected to create reasonably clear log files before asking a
	// debugging question on Edstem or OH. Use of this logger is optional, and
	// you are free to remove it completely.
	logger *log.Logger // We provide you with a separate logger per peer.

	ElectionTimeout int

	Interval int

	currentTerm int

	state State

	votedFor int

	applyCh chan ApplyCommand

	hearthbeat_signal chan int

	step_down_signal chan int

	logs []LogStruct

	commitIndex int

	lastApplied int

	// leader part
	nextIndex []int

	matchIndex []int

	vote_count    int
	leader_signal chan bool

	// TODO - Your data here (2A, 2B).
	// Look at the Raft paper's Figure 2 for a description of what
	// state a Raft peer should maintain
}

// GetState
// ==========
//
// Return "me", current term and whether this peer
// believes it is the leader
func (rf *Raft) GetState() (int, int, bool) {
	var me int
	var term int
	var isleader bool
	me = rf.me
	term = rf.getTerm()
	isleader = rf.stateInfo() == LEADER
	// TODO - Your code here (2A)
	return me, term, isleader
}

// RequestVoteArgs
// ===============
//
// Example RequestVote RPC arguments structure.
//
// # Please note: Field names must start with capital letters!
type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
	// TODO - Your data here (2A, 2B)
}

// RequestVoteReply
// ================
//
// Example RequestVote RPC reply structure.
//
// # Please note: Field names must start with capital letters!
type RequestVoteReply struct {
	Term        int
	VoteGranted bool

	// TODO - Your data here (2A)
}
type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogStruct
	LeaderCommit int
}
type AppendEntriesReply struct {
	Term    int
	Success bool
}

// RequestVote
// ===========
//
// Example RequestVote RPC handler
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// TODO - Your code here (2A, 2B)
	// step down when find higher term
	if args.Term < rf.getTerm() {
		reply.Term = rf.getTerm()
		reply.VoteGranted = false
		return
	}
	if args.Term > rf.getTerm() && (rf.stateInfo() == CANDIDATE || rf.stateInfo() == LEADER) {
		rf.setTerm(args.Term)
		rf.setVotedFor(-1)
		rf.logger.Println("In request vote down level to Follower")
		rf.step_down_signal <- args.Term
		reply.VoteGranted = false

		reply.Term = rf.getTerm()

		////reply.VoteGranted = false
		//reply.Term = rf.getTerm()
		//return
	}

	if args.Term > rf.getTerm() && rf.stateInfo() == FOLLOWER {
		rf.setTerm(args.Term)
		rf.setVotedFor(args.CandidateId)
		rf.logger.Println("Follower change term to ", rf.getTerm())
		rf.setVotedFor(args.CandidateId)

		rf.logger.Printf("Follower %d vote to candidate id %d\n", rf.me, args.CandidateId)
		lastIndex, lastTerm := rf.getLastLogInfo()

		if lastTerm > args.LastLogTerm || (lastTerm == args.LastLogTerm && lastIndex > args.LastLogIndex) {
			reply.VoteGranted = false
			rf.logger.Println("vote false")

		} else {
			reply.VoteGranted = true
			rf.hearthbeat_signal <- 1

			rf.logger.Println("vote correct")

		}

		reply.Term = rf.getTerm()

		return
	}

	//&& rf.stateInfo() == FOLLOWER
	//&& (rf.getVoteFor() == -1 || rf.getVoteFor() == args.CandidateId)

	reply.VoteGranted = false

	reply.Term = rf.getTerm()

	return

}

// sendRequestVote
// ===============
//
// Example code to send a RequestVote RPC to a server.
//
// server int -- index of the target server in
// rf.peers[]
//
// args *RequestVoteArgs -- RPC arguments in args
//
// reply *RequestVoteReply -- RPC reply
//
// The types of args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers)
//
// The rpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost
//
// Call() sends a request and waits for a reply.
//
// If a reply arrives within a timeout interval, Call() returns true;
// otherwise Call() returns false
//
// Thus Call() may not return for a while.
//
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply
//
// Call() is guaranteed to return (perhaps after a delay)
// *except* if the handler function on the server side does not return
//
// Thus there
// is no need to implement your own timeouts around Call()
//
// Please look at the comments and documentation in ../rpc/rpc.go
// for more details
//
// If you are having trouble getting RPC to work, check that you have
// capitalized all field names in the struct passed over RPC, and
// that the caller passes the address of the reply struct with "&",
// not the struct itself
//func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) {
//	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
//	//rf.logger.Println(*reply == RequestVoteReply{})
//	rf.logger.Println("vote state is ", reply.VoteGranted)
//
//	if ok && rf.stateInfo() == CANDIDATE && reply.VoteGranted {
//		rf.setVoteCount()
//		//if rf.getVoteCount() > len(rf.peers)/2 {
//		//	rf.leader_signal <- true
//		//
//		//}
//	}
//
//}

func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply, vote_signal chan bool) {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	//rf.logger.Println(*reply == RequestVoteReply{})
	//rf.logger.Println("ok state is ", ok)

	if !ok {
		vote_signal <- false
		rf.logger.Println("server down")
		return
	}

	//if reply.Term > rf.getTerm() {
	//	vote_signal <- reply.VoteGranted
	//	// may cause problem
	//
	//	rf.setTerm(reply.Term)
	//	rf.step_down_signal <- reply.Term
	//	return
	//
	//}
	vote_signal <- reply.VoteGranted
	return

}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	//rf.logger.Println("The Follower state is Term:", rf.getTerm(), "state", rf.stateInfo(), "Length", len(args.Entries))

	if args.Term < rf.getTerm() {
		reply.Term = rf.getTerm()
		reply.Success = false
		return

	}
	if args.Term >= rf.getTerm() && rf.stateInfo() == FOLLOWER {
		rf.setTerm(args.Term)
		rf.hearthbeat_signal <- 1
	}
	//rf.logger.Println("The Follower state is Term:", rf.getTerm(), "state", rf.stateInfo(), "Length", len(args.Entries))

	if args.Term >= rf.getTerm() && rf.stateInfo() == CANDIDATE {
		rf.setTerm(args.Term)
		rf.step_down_signal <- args.Term
	}
	if args.Term > rf.getTerm() && rf.stateInfo() == LEADER {
		rf.setTerm(args.Term)
		rf.step_down_signal <- args.Term

	}
	if rf.getLength() < args.PrevLogIndex {
		rf.logger.Println("Problem with PrevLogIndex", args.PrevLogIndex)

		reply.Term = rf.getTerm()

		reply.Success = false
		return
	} else {
		rf.logger.Println("In outter", args.PrevLogIndex)
		if rf.getLogTerm(args.PrevLogIndex) == args.PrevLogTerm {

			reply.Term = rf.getTerm()

			reply.Success = true

			rf.setEntries(args.Entries, args.PrevLogIndex)

		} else {
			//rf.logger.Println("id ", rf.me, "received", args.Entries)
			//rf.logger.Println("argIndex is ", args.PrevLogIndex, "args PrevLogTerm is ", args.PrevLogTerm)
			//rf.logger.Println("remove entries")
			//rf.logger.Println("The rf is ", rf.logs)

			rf.removeEntries(args.PrevLogIndex - 1)

			//rf.logger.Println("after is ", rf.logs)
			//rf.logger.Println("removed entries is ", rf.logs)
			reply.Term = rf.getTerm()
			reply.Success = false
		}

	}

	//rf.logger.Println("The Follower state is Term:", rf.getTerm(), "state", rf.stateInfo(), "Length", len(args.Entries))
	//rf.logger.Println("The LeaderCommit is ", args.LeaderCommit)
	//rf.logger.Println("The rfCommit is ", rf.getCommitIndex())
	if args.LeaderCommit > rf.getCommitIndex() {
		current_i, _ := rf.getLastLogInfo()
		rf.setCommitIndex(min(args.LeaderCommit, current_i))
		go rf.ProcessCommit()

	}

}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	//rf.logger.Println(*reply == AppendEntriesReply{})

	if ok {

		if reply.Term > rf.getTerm() {
			rf.logger.Println("Send downlevel")

			rf.step_down_signal <- reply.Term
			return
		}

		if reply.Success {
			rf.mux.Lock()
			if len(rf.logs) > 0 {
				rf.matchIndex[server] = rf.logs[len(rf.logs)-1].CommandInfo.Index
			}
			N := rf.matchIndex[server]
			rf.nextIndex[server] = N + 1

			if N > rf.commitIndex {
				count := 0
				for _, match := range rf.matchIndex {
					if match >= N && rf.logs[N-1].Term == rf.currentTerm {
						count++
					}
				}

				if count > len(rf.peers)/2 {
					rf.commitIndex = N

				}

			}

			rf.mux.Unlock()

			go rf.ProcessCommit()

		} else {
			rf.mux.Lock()
			rf.nextIndex[server]--
			//next_index := rf.nextIndex[server]
			//prev_term := -1
			//if next_index-2 >= 0 {
			//	prev_term = rf.logs[next_index-2].Term
			//}

			//prev_term = rf.logs[next_index-2].Term
			//commit := rf.commitIndex
			//entries := rf.logs[next_index-1:]
			//args := AppendEntriesArgs{Term: rf.currentTerm, LeaderId: rf.me, PrevLogIndex: next_index - 1, PrevLogTerm: prev_term, LeaderCommit: commit, Entries: entries}
			//reply := AppendEntriesReply{}
			rf.mux.Unlock()
			//go rf.sendAppendEntries(server, &args, &reply)

		}

	}

}

// PutCommand
// =====
//
// The service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log
//
// If this server is not the leader, return false.
//
// Otherwise start the agreement and return immediately.
//
// There is no guarantee that this command will ever be committed to
// the Raft log, since the leader may fail or lose an election
//
// The first return value is the index that the command will appear at
// if it is ever committed
//
// The second return value is the current term.
//
// The third return value is true if this server believes it is
// the leader
func (rf *Raft) PutCommand(command interface{}) (int, int, bool) {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	index := -1
	term := -1
	isLeader := rf.state == LEADER

	if !isLeader {
		return index, term, isLeader
	}

	rf.logger.Println("PutCommand", command)
	rf.logger.Println("The leader id is", rf.me)

	term = rf.currentTerm
	index = len(rf.logs) + 1
	new_log := LogStruct{Term: term, CommandInfo: ApplyCommand{Command: command, Index: index}}
	rf.logs = append(rf.logs, new_log)
	rf.logger.Println("Print log", rf.logs)
	rf.matchIndex[rf.me] = len(rf.logs)
	rf.nextIndex[rf.me] = len(rf.logs) + 1

	// TODO - Your code here (2B)
	return index, term, isLeader
}

// Stop
// ====
//
// The tester calls Stop() when a Raft instance will not
// be needed again
//
// You are not required to do anything
// in Stop(), but it might be convenient to (for example)
// turn off debug output from this instance
func (rf *Raft) Stop() {
	// TODO - Your code here, if desired
}
func (rf *Raft) initState(state State) {
	rf.mux.Lock()
	defer rf.mux.Unlock()

	rf.state = state
	rf.votedFor = -1
	rf.vote_count = 0

}
func (rf *Raft) initLeaderState() {
	rf.mux.Lock()
	defer rf.mux.Unlock()

	rf.state = LEADER
	rf.votedFor = -1
	rf.vote_count = 0
	index := 0

	if len(rf.logs) > 0 {
		index = rf.logs[len(rf.logs)-1].CommandInfo.Index

	}
	rf.nextIndex = make([]int, len(rf.peers))
	for i := range rf.nextIndex {
		rf.nextIndex[i] = index + 1
	}

	rf.matchIndex = make([]int, len(rf.peers))
	for i := range rf.matchIndex {
		rf.matchIndex[i] = 0
	}

}

func (rf *Raft) ProcessFollowerState() {

	rf.initState(FOLLOWER)
	//rf.Clear()
	// reset data

	for {

		select {
		case <-rf.hearthbeat_signal:
			//rf.logger.Println("get heartbeat")
		case <-time.After(time.Duration(rf.ElectionTimeout) * time.Millisecond):

			go rf.ProcessCandidateState()
			rf.logger.Printf("Follower %d to Candidate", rf.me)
			return

		}
	}

}

func (rf *Raft) ProcessCandidateState() {
	rf.initState(CANDIDATE)
	//rf.Clear()

	rf.setVotedFor(rf.me)
	t := rf.getTerm() + 1
	rf.setTerm(t)
	leader_signal := make(chan bool)
	//current_signal := make(chan bool)
	go rf.ProcessElection(rf.getTerm(), t, leader_signal)
	rf.logger.Println("Process Election id is", t)

	select {

	case <-leader_signal:
		rf.logger.Printf("Candidate %d to Leader", rf.me)
		go rf.ProcessLeader()
		return

	case term := <-rf.step_down_signal:
		rf.setTerm(term)

		rf.logger.Printf("Candidate %d to follower", rf.me)
		go rf.ProcessFollowerState()
		return

	case <-time.After(time.Duration(rf.ElectionTimeout) * time.Millisecond):
		rf.logger.Printf("Candidate %d reelection", rf.me)

		go rf.ProcessCandidateState()

		return

	}

}

//	func (rf *Raft) ProcessElection(Term int, pid int, current_signal chan bool) {
//		lastIndex, lastTerm := rf.getLastLogInfo()
//		count := 1
//		for peer := range rf.peers {
//			if peer != rf.me {
//				reply := RequestVoteReply{}
//				args := RequestVoteArgs{CandidateId: rf.me, Term: Term, LastLogIndex: lastIndex, LastLogTerm: lastTerm}
//
//				go rf.sendRequestVote(peer, &args, &reply)
//
//			}
//		}
//
// }
func (rf *Raft) ProcessElection(Term int, pid int, leader_signal chan bool) {

	vote_signal := make(chan bool, len(rf.peers)-1)
	lastIndex, lastTerm := rf.getLastLogInfo()
	for peer := range rf.peers {
		if peer != rf.me {
			reply := RequestVoteReply{}
			args := RequestVoteArgs{CandidateId: rf.me, Term: Term, LastLogIndex: lastIndex, LastLogTerm: lastTerm}
			rf.logger.Println(args)

			go rf.sendRequestVote(peer, &args, &reply, vote_signal)

		}
	}
	vote_count := 1
	vote_success := false

	for i := 0; i < len(rf.peers)-1; i++ {
		//rf.logger.Printf("Election pid is %d Vote Pending\n", pid)
		isVoted := <-vote_signal

		if isVoted && !vote_success {
			//rf.logger.Println("Pid is", pid)

			vote_count++

			if vote_count > len(rf.peers)/2 {
				leader_signal <- true
				vote_success = true
				close(leader_signal)
				//rf.logger.Println("Leader selected")

			}

		}

	}

}

func (rf *Raft) ProcessLeader() {
	rf.initLeaderState()
	//rf.Clear()

	for peer := range rf.peers {
		if peer != rf.me {
			next_index := rf.getNextIndex(peer)
			prev_term := rf.getLogTerm(next_index - 1)

			commit := rf.getCommitIndex()
			args := AppendEntriesArgs{Term: rf.getTerm(), LeaderId: rf.me, PrevLogIndex: next_index - 1, PrevLogTerm: prev_term, LeaderCommit: commit}
			reply := AppendEntriesReply{}
			rf.logger.Println("Send entries")
			go rf.sendAppendEntries(peer, &args, &reply)
		}
	}

	ticker := time.NewTicker(time.Duration(rf.Interval) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:

			last, _ := rf.getLastLogInfo()
			for peer := range rf.peers {
				if peer != rf.me {
					nextIndex := rf.getNextIndex(peer)
					if last >= nextIndex {
						prev_term := rf.getLogTerm(nextIndex - 1)

						commit := rf.getCommitIndex()
						entries := rf.getEntries(nextIndex - 1)

						args := AppendEntriesArgs{Term: rf.getTerm(), LeaderId: rf.me, PrevLogIndex: nextIndex - 1, PrevLogTerm: prev_term, LeaderCommit: commit, Entries: entries}
						reply := AppendEntriesReply{}
						go rf.sendAppendEntries(peer, &args, &reply)

					} else {
						// heartbeat case
						prev_term := rf.getLogTerm(nextIndex - 1)

						commit := rf.getCommitIndex()
						args := AppendEntriesArgs{Term: rf.getTerm(), LeaderId: rf.me, PrevLogIndex: nextIndex - 1, PrevLogTerm: prev_term, LeaderCommit: commit}
						reply := AppendEntriesReply{}
						go rf.sendAppendEntries(peer, &args, &reply)

					}

				}

			}

		case term := <-rf.step_down_signal:
			rf.setTerm(term)

			rf.logger.Println("Leader case: new "+
				"Term is", term)
			rf.logger.Println("Leader to follower")
			go rf.ProcessFollowerState()
			return

		}

	}

}

func (rf *Raft) ProcessCommit() {
	rf.mux.Lock()
	for rf.commitIndex > rf.lastApplied {

		rf.lastApplied++
		logEntries := rf.logs[rf.lastApplied-1]
		rf.logger.Println("The entries is ", rf.logs)
		rf.logger.Println("The lastApplied is ", rf.lastApplied)
		rf.logger.Println("The commitIndex is", rf.commitIndex)

		rf.applyCh <- ApplyCommand{Index: logEntries.CommandInfo.Index, Command: logEntries.CommandInfo.Command}

	}
	rf.mux.Unlock()

}

func (rf *Raft) Clear() {
	for {
		select {
		case <-rf.hearthbeat_signal:
		case <-rf.step_down_signal:
		case <-rf.leader_signal:
		//case <-rf.vote_signal:
		//case <-rf.leader_signal:
		//case <-rf.stop_election_signal:
		default:
			return

		}
	}
}

// NewPeer
// ====
//
// The service or tester wants to create a Raft server.
//
// The port numbers of all the Raft servers (including this one)
// are in peers[]
//
// This server's port is peers[me]
//
// All the servers' peers[] arrays have the same order
//
// applyCh
// =======
//
// applyCh is a channel on which the tester or service expects
// Raft to send ApplyCommand messages
//
// NewPeer() must return quickly, so it should start Goroutines
// for any long-running work
func NewPeer(peers []*rpc.ClientEnd, me int, applyCh chan ApplyCommand) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.me = me

	if kEnableDebugLogs {
		peerName := peers[me].String()
		logPrefix := fmt.Sprintf("%s ", peerName)
		if kLogToStdout {
			rf.logger = log.New(os.Stdout, peerName, log.Lmicroseconds|log.Lshortfile)
		} else {
			err := os.MkdirAll(kLogOutputDir, os.ModePerm)
			if err != nil {
				panic(err.Error())
			}
			logOutputFile, err := os.OpenFile(fmt.Sprintf("%s/%s.txt", kLogOutputDir, logPrefix), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				panic(err.Error())
			}
			rf.logger = log.New(logOutputFile, logPrefix, log.Lmicroseconds|log.Lshortfile)
		}
		rf.logger.Println("logger initialized")
	} else {
		rf.logger = log.New(ioutil.Discard, "", 0)
	}

	rf.applyCh = applyCh

	rf.hearthbeat_signal = make(chan int)
	rf.votedFor = -1
	rf.currentTerm = 0

	rf.state = FOLLOWER
	rf.Interval = 150
	rf.ElectionTimeout = RandIntBetween(300, 600)
	//rf.vote_signal = make(chan bool)
	rf.step_down_signal = make(chan int)

	rf.applyCh = applyCh
	rf.commitIndex = 0
	rf.lastApplied = 0
	rf.logs = make([]LogStruct, 0)
	rf.vote_count = 0
	rf.leader_signal = make(chan bool)

	go rf.ProcessFollowerState()

	// TODO - Your initialization code here (2A, 2B)

	return rf
}

func RandIntBetween(min, max int) int {

	return min + rand.Intn(max-min)
}
func (rf *Raft) setVotedFor(candidateID int) {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.votedFor = candidateID

}
func (rf *Raft) getTerm() int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.currentTerm
}
func (rf *Raft) stateInfo() State {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.state
}
func (rf *Raft) setState(state State) {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.state = state
}
func (rf *Raft) setTerm(term int) {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.currentTerm = term

}
func (rf *Raft) getCommitIndex() int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.commitIndex
}
func (rf *Raft) setCommitIndex(i int) {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.commitIndex = i

}
func (rf *Raft) getLastLogInfo() (int, int) {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	index := 0
	term := -1

	if len(rf.logs) > 0 {
		index = rf.logs[len(rf.logs)-1].CommandInfo.Index
		term = rf.logs[len(rf.logs)-1].Term

	}

	return index, term
}

func (rf *Raft) getNextIndex(server int) int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.nextIndex[server]
}

func (rf *Raft) getLogTerm(index int) int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	term := -1
	if index > 0 {
		term = rf.logs[index-1].Term
	}
	return term
}
func (rf *Raft) getLogIndex(index int) int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	i := 0
	if index > 0 {
		i = rf.logs[index-1].CommandInfo.Index
	}
	return i
}
func (rf *Raft) getEntries(start int) []LogStruct {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.logs[start:]

}
func (rf *Raft) setEntries(entries []LogStruct, index int) {
	rf.mux.Lock()
	defer rf.mux.Unlock()

	rf.logs = append(rf.logs[:index], entries...)
	rf.logger.Println("After Change, entries is ", rf.logs)

}

func (rf *Raft) removeEntries(end int) {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.logs = rf.logs[:end]

}

func (rf *Raft) getLength() int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return len(rf.logs)

}

func (rf *Raft) getVoteFor() int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.votedFor

}

func (rf *Raft) getLastApplied() int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.lastApplied

}

func (rf *Raft) setLastApplied() {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.lastApplied++
}

func (rf *Raft) getVoteCount() int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.vote_count

}
func (rf *Raft) setVoteCount() {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.vote_count++
}
