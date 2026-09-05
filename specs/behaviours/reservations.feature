Feature: Shared vector reservations
  HIPPO admits independent repositories concurrently only inside one safe CPU-and-memory budget.

  @e2e-exempt
  Scenario: Automatic reservations divide capacity by profile owner shares
    Given healthy host capacity for automatic reservation planning
    When automatic reservations are calculated for every built-in profile
    Then balanced constrained and minimal use four two and one owner shares

  @e2e-exempt
  Scenario: Maximum-width automatic shares do not overflow ceiling division
    Given maximum representable CPU and memory capacity for balanced and constrained profiles
    When automatic fair-share reservations are calculated
    Then both vector dimensions retain their exact four-share and two-share ceilings

  @e2e-exempt
  Scenario: Every development class consumes reservation capacity
    Given enough capacity for one owner of each development class
    When service ephemeral and transactional sessions are admitted
    Then all three allocations appear in shared reservation totals

  @e2e-exempt
  Scenario: Explicit reservations may be smaller than automatic shares
    Given automatic allocation larger than the immutable floors
    When explicit CPU and memory reservations below that allocation are requested
    Then the smaller explicit vector is admitted unchanged

  @e2e-exempt
  Scenario: Reservation floors reject unsafe requests
    Given explicit requests below one CPU or 256 MiB
    When each unsafe reservation is validated
    Then each request requires replanning with exit 78 before enqueue

  @e2e-exempt
  Scenario: Vector admission is atomic
    Given remaining capacity fits only one dimension of a request
    When that CPU and memory vector requests admission
    Then neither dimension is reserved and no child starts

  @e2e-exempt
  Scenario: Impossible reservation requires replanning
    Given a reservation larger than total safe host capacity
    When impossible capacity is requested
    Then admission requires replanning with exit 78 instead of waiting

  @e2e-exempt
  Scenario: Temporary exhaustion uses the bounded wait
    Given a live owner temporarily consumes the remaining capacity
    When another fitting reservation waits through its deadline
    Then the waiter remains FIFO head through its deadline and is deferred with exit 75

  @e2e-exempt
  Scenario: FIFO head cannot be bypassed by a smaller request
    Given a large waiter is ahead of a smaller waiter
    When capacity becomes sufficient only for the smaller waiter
    Then neither waiter bypasses the live FIFO head

  @e2e-exempt
  Scenario: Exhausted FIFO sequence resets only after a stale epoch empties
    Given reservation epochs at the maximum FIFO sequence with live and stale identities
    When another reservation requests admission in each epoch
    Then the live epoch stays byte-unchanged while the stale empty epoch restarts safely

  @e2e-exempt
  Scenario: Inherited sessions reuse their fixed allocation
    Given a live reserved session with an inheritable token
    When a nested guarded command uses that token
    Then no second reservation is created and allocation cannot expand

  @e2e-exempt
  Scenario: Fixed allocations clamp concurrency mappings
    Given a session allocated two CPU units and 512 MiB
    When missing lower higher and malformed concurrency mappings are evaluated
    Then canonical values are fixed lower values survive higher values clamp and malformed values replan

  @e2e-exempt
  Scenario: HIPPO protocol names cannot be consumer concurrency mappings
    Given mappings for HIPPO root reserved memory config and default config plus one arbitrary worker name
    When every mapping is validated against a fixed reservation allocation
    Then every HIPPO mapping replans before child execution and the arbitrary worker mapping remains valid

  @e2e-exempt
  Scenario: Stale identity cannot retain or inherit capacity
    Given stale owner records including a reused diagnostic PID
    When reservation ownership is reconciled
    Then only a matching live process identity retains capacity or inheritance

  @e2e-exempt
  Scenario: Active exclusive compatibility blocks reservation takeover
    Given a live schema one exclusive compatibility session
    When reservation-mode admission is requested
    Then it defers with exit 75 without changing compatibility state

  @e2e-exempt
  Scenario: Malformed compatibility state remains fail closed during takeover
    Given malformed compatibility heavy state without a mode marker
    When reservation-mode admission inspects that state
    Then it defers with exit 75 and leaves the malformed state unchanged

  @e2e-exempt
  Scenario: Unsupported compatibility heavy-owner schema remains actionable
    Given compatibility heavy state with an unsupported owner schema
    When exclusive admission and the heavy-lease diagnostic inspect that state
    Then admission defers without mutation and the diagnostic reports private recovery guidance

  @e2e-exempt
  Scenario: Malformed compatibility service session remains fail closed during takeover
    Given an unverifiable compatibility service session record without a mode marker
    When reservation-mode admission inspects the service session
    Then it defers with exit 75 and leaves the service record unchanged

  @e2e-exempt
  Scenario: Unreadable compatibility session inventory blocks reservation takeover
    Given compatibility session inventory cannot be enumerated
    When reservation admission attempts to take over the shared root
    Then admission defers and preserves the session inventory with private recovery guidance

  @e2e-exempt
  Scenario: Failed stale heavy cleanup blocks reservation takeover
    Given positively stale compatibility heavy state cannot be removed
    When reservation admission attempts to take over the shared root
    Then admission defers without writing a reservation marker or changing heavy state

  @e2e-exempt
  Scenario: Host pressure thresholds remain authoritative
    Given vector capacity fits under a threshold-blocked host sample
    When reservation admission evaluates that sample
    Then threshold pressure defers work without changing the ledger

  @e2e-exempt
  Scenario: Shedding selects one newest revocable owner
    Given transactional service and ephemeral owners under critical pressure
    When a pressure victim is elected atomically
    Then only the newest ephemeral is selected before any service

  @e2e-exempt
  Scenario: Service shedding follows exhausted ephemeral owners
    Given service owners and no eligible ephemeral owner
    When a pressure victim is elected atomically
    Then only the newest service is selected

  @e2e-exempt
  Scenario: Remote shedding completes one victim before another election
    Given a selected remote ephemeral owner that ignores termination
    When its owning guard performs bounded shedding
    Then it escalates to a kill and no additional victim is selected before owned release

  @e2e-exempt
  Scenario: Remote observation never signals a replaced identity
    Given selected ownership disappears while its process group remains live
    When the selecting guard observes that disappearance
    Then no signal reaches the unowned process group and disappearance ends observation

  @e2e-exempt
  Scenario: Remote selectors never signal another owner's child
    Given a selected remote owner whose owning guard is temporarily unresponsive
    When the selecting guard observes that owner through a bounded deadline
    Then the remote child is not signaled and remains the global no-cascade barrier

  @e2e-exempt
  Scenario: Remote observation remains bounded by its deadline
    Given a selected remote owner and a held coordination mutation lock
    When the selecting guard waits through its observation deadline
    Then it returns boundedly without signaling and preserves the selected barrier

  @e2e-exempt
  Scenario: Shared-root contention defers instead of failing admitted work
    Given a peer holding the shared coordination lock while a guard activates and supervises its child
    When the guard activates its reservation and then samples through the held lock
    Then activation returns the retryable deferral exit and supervision keeps its healthy child

  @e2e-exempt
  Scenario: Owner cancellation remains bounded when release coordination is held
    Given a running reserved owner and a coordination mutation lock held by another goroutine in the same process
    When its owning guard is cancelled and reaps the child
    Then cleanup defers boundedly without a caller failure, with exact ledger bytes and an externally locked identity until a later atomic retry after lock release

  @e2e-exempt
  Scenario: Cancelled FIFO waiters use a fresh cleanup deadline
    Given a queued reservation waiter and a held coordination mutation lock
    When its acquisition context is cancelled before the lock is released
    Then bounded cleanup removes the waiter without blocking the FIFO queue

  @e2e-exempt
  Scenario: Failed cancelled-waiter cleanup retains verifiable FIFO ownership
    Given a queued reservation waiter whose coordination lock remains held past its fresh cleanup deadline
    When its acquisition context is cancelled and bounded cleanup cannot mutate the ledger
    Then the waiter identity and exact FIFO bytes remain and a following FIFO waiter admits only after automatic head cleanup

  @e2e-exempt
  Scenario: Supervisor death cannot release a live child reservation
    Given a compiled guard supervising a long-lived child in an isolated process group
    When only the guard is killed and reservation liveness is reconciled
    Then the running child keeps its reservation until the child group exits

  @e2e-exempt
  Scenario: Cancellation retains ownership when post-kill retirement is unconfirmed
    Given a reserved child whose TERM and KILL waits remain unconfirmed
    When its owning guard is cancelled
    Then cancellation returns boundedly while reservation and port competitors remain deferred until later retirement and then admit

  @e2e-exempt
  Scenario: Pressure shedding preserves its stable exit while retirement is unconfirmed
    Given selected reserved owners for storage and non-storage pressure whose KILL waits remain unconfirmed
    When each owning guard performs bounded shedding
    Then each returns exit 73 or 75 respectively while reservation and port competitors defer until retirement and then admit

  @e2e-exempt
  Scenario: Port ownership survives supervisor-only death
    Given a guarded child holding a real leased port through a kernel lifetime identity
    When only its HIPPO supervisor is killed
    Then a competitor cannot reclaim the port until the child group exits

  @e2e-exempt
  Scenario Outline: Tokenized port identity corruption remains fail closed
    Given a tokenized live port lease whose identity file is <fault>
    When another owner requests that port in the same supervisor process
    Then the existing marker is preserved and the port remains unavailable

    Examples:
      | fault    |
      | missing  |
      | replaced |

  @e2e-exempt
  Scenario: A stale same-process port handle cannot release a replacement lease
    Given an old and replacement port lease handle for the same process owner
    When the old handle attempts to release the replacement marker
    Then token authentication rejects the stale handle and preserves the replacement lease

  @e2e-exempt
  Scenario: Legacy tokenless port markers use conservative PID compatibility
    Given live and positively stale schema-one port markers without identity tokens
    When a v0.4 owner requests each legacy port
    Then the live PID remains unavailable and only the positively stale PID is reclaimed

  @e2e-exempt
  Scenario: A successful leader cannot retire a live background descendant
    Given a reserved leader that exits zero after spawning a background descendant without an inherited unknown file descriptor
    When the direct leader has been reaped
    Then reservation and port ownership remain until the process group retires and are later reclaimed safely

  @e2e-exempt
  Scenario: An inherited session cannot bypass background descendant retirement
    Given an inherited reserved session that spawns a background descendant without an inherited unknown file descriptor
    When the inherited direct leader exits zero
    Then its reservation and port evidence remain until the inherited process group retires

  @e2e-exempt
  Scenario: A stalled lifetime activation handshake is bounded
    Given a lifetime launcher that remains stopped before reporting payload activation
    When its owning guard reaches the activation deadline
    Then the guard returns boundedly without releasing the launcher's detectable reservation and port ownership

  @e2e-exempt
  Scenario Outline: Payloads cannot inherit lifetime identity descriptors
    Given a compiled <mode> reserved child with reservation and port identity locks
    When the payload enumerates descriptors and attempts to unlock every inherited file
    Then no private identity is observable or unlockable and competitors remain blocked through group retirement

    Examples:
      | mode      |
      | ordinary  |
      | inherited |

  @e2e-exempt
  Scenario: Failed activation reporting retains launcher-owned identity
    Given a payload leader exits after starting a no-identity-descriptor descendant but its launcher cannot report activation
    When failed-launch cleanup reaches its bounded retirement decision
    Then no payload inherits identity and reservation plus port evidence remain until positive group retirement

  @e2e-exempt
  Scenario: Payloads cannot forge the launcher activation report
    Given a compiled reserved payload and a launcher-owned activation channel
    When the payload writes a forged process group to the inherited report descriptor
    Then the descriptor is closed in the payload and only the launcher report reaches the supervisor

  @e2e-exempt
  Scenario Outline: Reservation identity path corruption remains fail closed
    Given a live reservation <record> whose token identity path is <fault>
    When status and competing admission reconcile that ledger
    Then exact owner capacity or FIFO position remains until the original identity retires and later reconciliation succeeds

    Examples:
      | record | fault    |
      | owner  | missing  |
      | owner  | replaced |
      | waiter | missing  |
      | waiter | replaced |

  @e2e-exempt
  Scenario: An embedded guard abandons only its local lifetime handles
    Given a long-lived caller whose guarded run returns before child-group retirement is confirmed
    When the lifetime holder retires later without the caller process exiting
    Then the same caller can safely reclaim both reservation capacity and the leased port

  @e2e-exempt
  Scenario: Hidden lifetime launch mode requires a parent capability
    Given a direct HIPPO invocation with the internal environment and launcher argument but no capability pipes
    When the public command boundary interprets the invocation
    Then no payload bypasses admission and the invocation is rejected as an ordinary invalid command

  @e2e-exempt
  Scenario Outline: Schema-one ownership survives supervisor-only death
    Given a compiled schema-one <class> guard with a live child group
    When only the compatibility supervisor is killed and ownership is reconciled
    Then reservation takeover remains deferred until the compatibility child group retires

    Examples:
      | class   |
      | heavy   |
      | service |

  @e2e-exempt
  Scenario: Schema-one embedded ownership retires independently of its caller PID
    Given a long-lived schema-one caller whose guarded group retirement is initially unconfirmed
    When its launcher and group later retire without the caller exiting
    Then compatibility ownership becomes reclaimable without treating the live caller PID as the owner

  @e2e-exempt
  Scenario Outline: Legacy schema-one PID-only ownership remains conservative
    Given live and positively stale zero-metadata legacy <class> ownership records
    When compatibility liveness and reservation takeover reconcile each shared root
    Then the live PID record is retained and defers takeover while only the positively stale record is reclaimed

    Examples:
      | class   |
      | heavy   |
      | service |

  @e2e-exempt
  Scenario: Owner-side shedding preserves the selected exit
    Given a reserved owner selected for storage shedding
    When its own guard observes the mark and terminates its child
    Then the child is reaped and the guard returns storage-blocked exit 73 before release

  @e2e-exempt
  Scenario: Transactional owners are never shed after admission
    Given only transactional owners remain under critical pressure
    When a pressure victim is elected atomically
    Then no victim is selected and new admission remains blocked

  Scenario: Schema two configuration preserves schema one compatibility
    Given valid schema one and schema two local configurations
    When both configuration documents are loaded
    Then schema one selects exclusive mode and schema two validates reservation limits

  Scenario: MiB inputs reject arithmetic overflow before conversion
    Given maximum representable and first overflowing MiB values for CLI and configuration
    When reservation memory inputs are parsed
    Then the maximum stays exact and overflow requires replanning before multiplication

  @e2e-exempt
  Scenario: Custom profiles inherit reservation shares and fixed concurrency
    Given a schema two custom profile extending constrained without an artificial concurrency cap
    When its automatic reservation and guarded summary are produced
    Then it inherits two shares and reports the fixed allocated CPU as concurrency

  @e2e-exempt
  Scenario: Shared owner limits remain conservative across configurations
    Given live owners and waiters with different maximum-owner settings
    When a looser configuration requests another reservation
    Then the strictest live shared-root owner limit remains authoritative

  @e2e-exempt
  Scenario: Active reservation capacity cannot shrink below live commitments
    Given a larger-capacity epoch with a live owner and FIFO waiter
    When a caller with a lower CPU or memory cap requests admission
    Then it defers without changing the epoch and the lower cap starts only after idle

  @e2e-exempt
  Scenario: Reservation ledger tokens are validated before mutation
    Given a reservation ledger with an invalid or duplicate owner token
    When reservation state is decoded by coordination
    Then admission fails closed and preserves the ledger bytes

  @e2e-exempt
  Scenario: Reservation ledger classes are validated before mutation
    Given a reservation ledger with an unknown workload class
    When reservation state is decoded by coordination
    Then admission fails closed and preserves the class-corrupt ledger bytes

  @e2e-exempt
  Scenario: Reservation ledger sequences are validated before mutation
    Given a reservation ledger with duplicate or nonmonotonic sequences
    When reservation state is decoded by coordination
    Then admission fails closed and preserves the sequence-corrupt ledger bytes

  @e2e-exempt
  Scenario: Reservation ledger vectors are validated before mutation
    Given a reservation ledger with negative or inconsistent resource vectors
    When reservation state is decoded by coordination
    Then admission fails closed and preserves the vector-corrupt ledger bytes

  @e2e-exempt
  Scenario: Reservation ledger arithmetic rejects overflow before mutation
    Given a reservation ledger whose valid individual vectors overflow aggregate arithmetic
    When reservation state is decoded by coordination
    Then admission fails closed and preserves the overflow-corrupt ledger bytes

  @e2e-exempt
  Scenario: Maximum-width admission cannot wrap vector capacity
    Given a live owner consumes maximum-width CPU and memory capacity
    When another minimum reservation requests admission
    Then admission defers without wrapping or changing allocated totals

  @e2e-exempt
  Scenario: Reservation ledger totals are validated before mutation
    Given a reservation ledger whose allocated totals exceed capacity
    When reservation state is decoded by coordination
    Then admission fails closed and preserves the total-corrupt ledger bytes

  @e2e-exempt
  Scenario: Reservation ledger structure is validated before mutation
    Given a reservation ledger with impossible owner and waiter structure
    When reservation state is decoded by coordination
    Then admission fails closed and preserves the structure-corrupt ledger bytes

  @e2e-exempt
  Scenario: Unknown identity errors never prune reservation accounting
    Given a live reservation whose identity probe returns an unknown filesystem error
    When status and admission reconcile reservation liveness
    Then both fail closed without changing bytes or exposing the identity

  @e2e-exempt
  Scenario: Missing live reservation accounting cannot initialize empty
    Given a reservation marker and live identity lock without its ledger
    When status and another admission inspect that root
    Then both fail closed without recreating or overbooking accounting

  Scenario: Status and development summaries expose schema four reservation totals
    Given privacy-safe reservation and development evidence
    When status and the lifetime summary are encoded
    Then schema four reports requested allocated waited and peak totals without private inputs

  @e2e-exempt
  Scenario: Status rejects overflowing aggregate waiter demand
    Given individually valid waiters whose CPU or memory demand overflows aggregate arithmetic
    When schema four reservation status aggregates the queued demand
    Then status fails closed with a privacy-safe coordination error

  @e2e-exempt
  Scenario: Development summaries retain the lifetime peak owner count
    Given a guarded child whose concurrent owner count rises after admission
    When reservation totals are sampled throughout supervision
    Then the schema four summary reports the highest observed owner count

  @e2e-exempt
  Scenario: Development summaries retain owners between host samples
    Given a guarded child with an overlapping owner shorter than one host sampling interval
    When the overlapping reservation is admitted and released during supervision
    Then the schema four summary includes that complete short-lived overlap

  @e2e-exempt
  Scenario: Reservation protocol files survive evidence retention
    Given aged live reservation waiter and coordination files
    When reservation evidence retention is enforced
    Then protocol files remain while expired evidence is removed

  @e2e-exempt
  Scenario: Atomic coordination writes survive evidence retention
    Given fresh actively-written coordination-marker and reservation-ledger temporary files
    When reservation evidence retention is enforced
    Then recognized atomic-write temporary files remain untouched

  @e2e-exempt
  Scenario: Abandoned atomic coordination temporary files expire
    Given expired orphaned coordination-marker and reservation-ledger temporary files
    When reservation evidence retention is enforced
    Then the orphaned temporary files are reclaimed without changing stable protocol files

  @e2e-exempt
  Scenario: Concurrent evidence disappearance does not abort guarded work
    Given two parallel reserved guards and concurrently finalized evidence entries
    When their evidence cleanup snapshots race with entry disappearance
    Then both commands complete without retry and their observed evidence remains
