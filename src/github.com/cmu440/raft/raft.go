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
// The RequestVoteReply type is used to store the response to a request for vote in a distributed
// consensus algorithm.
// @property {int} Term - The Term property represents the current term of the candidate requesting the
// vote. It is an integer value that is used to keep track of the current election term.
// @property {bool} VoteGranted - A boolean value indicating whether the vote has been granted or not.
type RequestVoteReply struct {
	Term        int
	VoteGranted bool

	// TODO - Your data here (2A)
}

// The `AppendEntriesArgs` type represents the arguments for an append entries RPC call in a
// distributed system.
// @property {int} Term - The current term of the leader sending the AppendEntries RPC.
// @property {int} LeaderId - The LeaderId property represents the unique identifier of the leader in a
// distributed system.
// @property {int} PrevLogIndex - PrevLogIndex is the index of the log entry immediately preceding the
// new entries being sent in the AppendEntries RPC.
// @property {int} PrevLogTerm - PrevLogTerm is the term of the log entry immediately preceding the new
// log entries being sent in the AppendEntries RPC.
// @property {[]LogStruct} Entries - The `Entries` property is a slice of `LogStruct` objects. It
// represents the log entries that the leader wants to append to the follower's log.
// @property {int} LeaderCommit - LeaderCommit is the index of the highest log entry known to be
// committed by the leader.
type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogStruct
	LeaderCommit int
}

// The AppendEntriesReply type represents a response to an append entries request in the Go programming
// language.
// @property {int} Term - The "Term" property represents the term of the leader that sent the
// AppendEntries RPC (Remote Procedure Call). It is an integer value that is used to ensure that
// followers are up to date and do not fall behind in terms of the leader's log.
// @property {bool} Success - A boolean value indicating whether the AppendEntries RPC request was
// successful or not. If Success is true, it means that the log entries were successfully appended to
// the receiver's log. If Success is false, it means that the receiver rejected the log entries due to
// inconsistencies or other reasons.
type AppendEntriesReply struct {
	Term    int
	Success bool
}

// RequestVote
// ===========
//
// Example RequestVote RPC handler
// The above code is implementing the RequestVote RPC method for a Raft node in a distributed consensus
// algorithm.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// TODO - Your code here (2A, 2B)

	// step down when find higher term
	if args.Term < rf.getTerm() {
		reply.Term = rf.getTerm()
		reply.VoteGranted = false
		return
	}
	if args.Term > rf.getTerm() && (rf.stateInfo() == LEADER || rf.stateInfo() == CANDIDATE) {
		rf.setTerm(args.Term)
		rf.setVotedFor(-1)
		rf.step_down_signal <- 1
	}
	if args.Term > rf.getTerm() && rf.stateInfo() == FOLLOWER {
		rf.logger.Println("Follower change term to ", rf.getTerm())
		rf.setTerm(args.Term)
		rf.setVotedFor(-1)
	}

	if (rf.getVoteFor() == -1 || rf.getVoteFor() == args.CandidateId) && rf.stateInfo() == FOLLOWER {

		rf.logger.Printf("Follower %d vote to candidate id %d\n", rf.me, args.CandidateId)
		lastIndex, lastTerm := rf.getLastLogInfo()

		if lastTerm < args.LastLogTerm || (lastTerm == args.LastLogTerm && lastIndex <= args.LastLogIndex) {
			reply.VoteGranted = true
			rf.setVotedFor(args.CandidateId)
			rf.hearthbeat_signal <- 1

			rf.logger.Println("vote correct")

		} else {
			reply.VoteGranted = false

			rf.logger.Println("vote false")

		}

		reply.Term = rf.getTerm()

		return
	}

	//&& rf.stateInfo() == FOLLOWER
	//&& (rf.getVoteFor() == -1 || rf.getVoteFor() == args.CandidateId)

	reply.VoteGranted = false

	reply.Term = rf.getTerm()

	return

	//&& rf.stateInfo() == FOLLOWER
	//&& (rf.getVoteFor() == -1 || rf.getVoteFor() == args.CandidateId)

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

// The above code is implementing the `AppendEntries` method for a Raft consensus algorithm in Go. This
// method is called by the leader to replicate log entries on the followers.
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	//rf.logger.Println("The Follower state is Term:", rf.getTerm(), "state", rf.stateInfo(), "Length", len(args.Entries))

	if args.Term < rf.getTerm() {
		reply.Term = rf.getTerm()
		reply.Success = false
		return

	}
	if rf.stateInfo() == FOLLOWER {
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
			if args.LeaderCommit > rf.getCommitIndex() {
				//current_i, _ := rf.getLastLogInfo()
				rf.setCommitIndex(args.LeaderCommit)
				go rf.ProcessCommit()

			}

		} else {

			reply.Term = rf.getTerm()
			reply.Success = false
		}

	}

	//rf.logger.Println("The Follower state is Term:", rf.getTerm(), "state", rf.stateInfo(), "Length", len(args.Entries))
	//rf.logger.Println("The LeaderCommit is ", args.LeaderCommit)
	//rf.logger.Println("The rfCommit is ", rf.getCommitIndex())

} // The above code is implementing the `sendAppendEntries` method for the Raft consensus algorithm in
// Go. This method is responsible for sending AppendEntries RPCs to other servers in the cluster.

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
			} else {
				rf.matchIndex[server] = 0
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

			if rf.getNextIndex(server) > 1 && rf.stateInfo() == LEADER {
				rf.mux.Lock()

				rf.nextIndex[server]--
				next_index := rf.nextIndex[server]
				prev_term := -1
				if next_index-2 >= 0 {
					prev_term = rf.logs[next_index-2].Term
				}
				entries := rf.logs[next_index-1:]
				args := AppendEntriesArgs{Term: args.Term, LeaderId: args.LeaderId, PrevLogIndex: next_index - 1, PrevLogTerm: prev_term, LeaderCommit: args.LeaderCommit, Entries: entries}
				reply := AppendEntriesReply{}
				rf.mux.Unlock()

				go rf.sendAppendEntries(server, &args, &reply)

			}
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

// The above code is defining a method called "initState" for the "Raft" struct in the Go programming
// language. This method takes a parameter called "state" of type "State".
func (rf *Raft) initState(state State) {
	rf.mux.Lock()
	defer rf.mux.Unlock()

	rf.state = state
	rf.votedFor = -1
	rf.commitIndex = 0
	rf.lastApplied = 0

}

// The above code is initializing the state of a Raft leader. It sets the Raft's state to LEADER,
// resets the votedFor field to -1, and sets the commitIndex and lastApplied fields to 0. It then
// determines the index of the last log entry, and initializes the nextIndex and matchIndex arrays for
// each peer in the Raft cluster. The nextIndex array is initialized to the index of the last log entry
// plus 1, and the matchIndex array is initialized to 0 for each peer.
func (rf *Raft) initLeaderState() {
	rf.mux.Lock()
	defer rf.mux.Unlock()

	rf.state = LEADER
	rf.votedFor = -1
	rf.commitIndex = 0
	rf.lastApplied = 0
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

// The above code is implementing the behavior of a follower in a Raft consensus algorithm.
func (rf *Raft) ProcessFollowerState() {

	rf.initState(FOLLOWER)
	rf.Clear()
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

// The above code is implementing the logic for the candidate state in the Raft consensus algorithm.
func (rf *Raft) ProcessCandidateState() {
	rf.initState(CANDIDATE)
	rf.Clear()

	rf.setVotedFor(rf.me)
	t := rf.getTerm() + 1
	rf.setTerm(t)
	lastIndex, lastTerm := rf.getLastLogInfo()
	vote_count := 1
	vote_signal := make(chan bool)
	for peer := range rf.peers {
		if peer != rf.me {
			reply := RequestVoteReply{}
			args := RequestVoteArgs{CandidateId: rf.me, Term: t, LastLogIndex: lastIndex, LastLogTerm: lastTerm}

			go rf.sendRequestVote(peer, &args, &reply, vote_signal)

		}
	}

	for {
		//rf.logger.Printf("Election pid is %d Vote Pending\n", pid)
		select {
		case term := <-rf.step_down_signal:
			rf.setTerm(term)
			rf.setVotedFor(-1)

			rf.logger.Printf("Candidate %d to follower", rf.me)
			go rf.ProcessFollowerState()
			return

		case isVoted := <-vote_signal:
			if isVoted {

				vote_count++

				if vote_count > len(rf.peers)/2 {
					go rf.ProcessLeader()
					return

					//rf.logger.Println("Leader selected")

				}

			}
		case <-time.After(time.Duration(rf.ElectionTimeout) * time.Millisecond):
			rf.logger.Printf("Candidate %d reelection", rf.me)

			go rf.ProcessCandidateState()

			return

		}

	}

}

// The above code is a method in the Raft struct that is used to send a RequestVote RPC to a specific
// server. It takes in the server index, the arguments for the RequestVote RPC, a reply struct to store
// the response, and a vote_signal channel to signal whether the vote was granted or not.

func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply, vote_signal chan bool) {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	//rf.logger.Println(*reply == RequestVoteReply{})
	//rf.logger.Println("ok state is ", ok)

	if ok {
		if reply.Term > rf.getTerm() {
			rf.step_down_signal <- reply.Term
			return
		}

		vote_signal <- reply.VoteGranted
	}

}

// The above code is implementing the leader state of a Raft consensus algorithm. It initializes the
// leader state, clears any previous state, and then sends AppendEntries RPCs to all other peers in the
// cluster. It also sets up a ticker to periodically send heartbeat messages to the followers. If it
// receives a step down signal (indicating that it should transition to a follower state), it updates
// its term and transitions to the follower state.
func (rf *Raft) ProcessLeader() {
	rf.initLeaderState()
	rf.Clear()

	for peer := range rf.peers {
		if peer != rf.me {
			next_index := rf.getNextIndex(peer)
			prev_term := rf.getLogTerm(next_index - 1)
			entries := rf.getEntries(next_index - 1)

			commit := rf.getCommitIndex()
			args := AppendEntriesArgs{Term: rf.getTerm(), LeaderId: rf.me, PrevLogIndex: next_index - 1, PrevLogTerm: prev_term, LeaderCommit: commit, Entries: entries}
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

// The above code is implementing the `ProcessCommit` function for a Raft consensus algorithm.

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

// The above code is defining a method called "Clear" for the Raft struct in the Go programming
// language. This method is used to clear any pending signals in the Raft instance.
func (rf *Raft) Clear() {
	for {
		select {
		case <-rf.hearthbeat_signal:
		case <-rf.step_down_signal:
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
// The function initializes a new Raft peer with the given parameters and starts a goroutine to process
// the follower state.
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
	rf.Interval = 100
	rf.ElectionTimeout = RandIntBetween(300, 500)
	//rf.vote_signal = make(chan bool)
	rf.step_down_signal = make(chan int)

	rf.applyCh = applyCh
	rf.commitIndex = 0
	rf.lastApplied = 0
	rf.logs = make([]LogStruct, 0)

	go rf.ProcessFollowerState()

	// TODO - Your initialization code here (2A, 2B)

	return rf
}

// The RandIntBetween function generates a random integer between a given minimum and maximum value.
func RandIntBetween(min, max int) int {

	return min + rand.Intn(max-min)
}

// The above code is defining a method called `setVotedFor` for the `Raft` struct in the Go programming
// language. This method takes an integer parameter `candidateID` and is used to set the `votedFor`
// field of the `Raft` struct to the value of `candidateID`. The method is protected by a mutex to
// ensure thread safety.
func (rf *Raft) setVotedFor(candidateID int) {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.votedFor = candidateID

}

// The above code is defining a method called `getTerm()` for the `Raft` struct in the Go programming
// language. This method is used to retrieve the current term of the `Raft` instance. It acquires a
// lock on the `rf` object, reads the value of the `currentTerm` field, and then releases the lock
// before returning the value.
func (rf *Raft) getTerm() int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.currentTerm
}

// The above code is defining a method called `stateInfo` for the `Raft` struct in the Go programming
// language. This method returns the current state of the `Raft` object. It acquires a lock on the `rf`
// object using a mutex to ensure thread safety, and then returns the `state` field of the `rf` object.
func (rf *Raft) stateInfo() State {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.state
}

// The above code is defining a method called `setState` for the `Raft` struct in the Go programming
// language. This method takes a parameter `state` of type `State` and is used to set the `state` field
// of the `Raft` struct to the provided `state` value. The method is using a mutex to ensure thread
// safety while modifying the `state` field.
func (rf *Raft) setState(state State) {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.state = state
}

// The above code is defining a method called `setTerm` for the `Raft` struct in the Go programming
// language. This method takes an integer parameter called `term` and is used to set the `currentTerm`
// field of the `Raft` struct to the value of `term`. The method is protected by a mutex to ensure
// thread safety.
func (rf *Raft) setTerm(term int) {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.currentTerm = term

}

// The above code is defining a method called `getCommitIndex()` for the `Raft` struct in the Go
// programming language. This method is used to retrieve the value of the `commitIndex` field of the
// `Raft` struct. It acquires a lock on the `Raft` struct using a mutex, reads the value of
// `commitIndex`, and then releases the lock before returning the value.
func (rf *Raft) getCommitIndex() int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.commitIndex
}

// The above code is defining a method called `setCommitIndex` for the `Raft` struct in the Go
// programming language. This method takes an integer `i` as a parameter and is used to set the
// `commitIndex` field of the `Raft` struct to the value of `i`. The method is protected by a mutex to
// ensure thread safety.
func (rf *Raft) setCommitIndex(i int) {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.commitIndex = i

}

// The above code is defining a method called `getLastLogInfo` for the `Raft` struct in Go. This method
// returns the index and term of the last log entry in the `logs` slice of the `Raft` struct. If the
// `logs` slice is not empty, it retrieves the index and term of the last log entry. If the `logs`
// slice is empty, it returns 0 for the index and -1 for the term.
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

// The above code is defining a method called `getNextIndex` for a struct type `Raft`. This method
// takes an integer parameter `server` and returns an integer value.
func (rf *Raft) getNextIndex(server int) int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.nextIndex[server]
}

// The above code is defining a method called `getLogTerm` for the `Raft` struct in Go. This method
// takes an `index` parameter and returns the term of the log entry at that index.

func (rf *Raft) getLogTerm(index int) int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	term := -1
	if index > 0 {
		term = rf.logs[index-1].Term
	}
	return term
}

// The above code is defining a method called `getLogIndex` for the `Raft` struct in Go. This method
// takes an `index` parameter and returns the index of the log entry at that position.
func (rf *Raft) getLogIndex(index int) int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	i := 0
	if index > 0 {
		i = rf.logs[index-1].CommandInfo.Index
	}
	return i
}

// The above code is a method implementation in the Raft struct in the Go programming language. It is
// called `getEntries` and takes an integer `start` as a parameter.
func (rf *Raft) getEntries(start int) []LogStruct {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.logs[start:]

}

// The above code is defining a method called "setEntries" for the Raft struct in the Go programming
// language. This method takes in a slice of LogStruct objects called "entries" and an integer called
// "index".
func (rf *Raft) setEntries(entries []LogStruct, index int) {
	rf.mux.Lock()
	defer rf.mux.Unlock()

	rf.logs = append(rf.logs[:index], entries...)
	rf.logger.Println("After Change, entries is ", rf.logs)

}

// The above code is defining a method called `removeEntries` for the `Raft` struct in Go. This method
// takes an integer parameter `end`.

func (rf *Raft) removeEntries(end int) {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.logs = rf.logs[:end]

}

// The above code is defining a method called `getLength()` for the `Raft` struct in the Go programming
// language. This method returns the length of the `logs` slice in the `Raft` struct. It uses a mutex
// to ensure thread safety when accessing the `logs` slice.
func (rf *Raft) getLength() int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return len(rf.logs)

}

// The above code is defining a method called `getVoteFor()` for the `Raft` struct in the Go
// programming language. This method is used to retrieve the value of the `votedFor` field in the
// `Raft` struct. The `votedFor` field represents the candidate ID that this Raft server has voted for
// in the current term. The method acquires a lock on the `Raft` struct to ensure thread safety and
// then returns the value of `votedFor`.
func (rf *Raft) getVoteFor() int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.votedFor

}

// The above code is defining a method called `getLastApplied` for the `Raft` struct in the Go
// programming language. This method returns the value of the `lastApplied` field of the `Raft` struct.
// The method is using a mutex to ensure thread safety when accessing the `lastApplied` field.
func (rf *Raft) getLastApplied() int {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	return rf.lastApplied

}

// The above code is defining a method called `setLastApplied` for the `Raft` struct in the Go
// programming language. This method is used to increment the `lastApplied` field of the `Raft` struct
// by 1. The method is protected by a mutex to ensure thread safety.
func (rf *Raft) setLastApplied() {
	rf.mux.Lock()
	defer rf.mux.Unlock()
	rf.lastApplied++
}
