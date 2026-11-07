# Jak tam to słowo

A vocabulary app for two learners of Polish and Russian. They share one collection of cards, study the cards in both directions and race each other in competitions.

## Language

### Study

**Card on screen**: The card that the study screen shows. It is the reviewed card while its review is pending or its feedback shows, and otherwise the first card of the queue.

### Competitions

**Competition**: One race of the two learners to the target score, from the invitation to the win or the cancellation. A rematch is a new competition. The code calls the current competition a round. _Avoid_: match, game.

**Race direction**: The translation direction that the inviter picks for a competition. It stays the same until the competition ends. Only answers in this direction count.

**Race training**: Study inside the Zawody tab in the race direction. It is the only place where answers count for the competition.

**Lifecycle action**: An invitation, an acceptance or a cancellation of a competition. An invitation carries the race direction.

**Unconfirmed action**: A lifecycle action whose result did not reach the app. A later read of the current competition confirms it, or the learner retries it for the same competition. _Avoid_: lost write.

**Competition sync**: The app's copy of the current competition for the signed-in learner, kept up to date with the server, together with the lifecycle actions that the learner can start now.

**Competition list**: All competitions of the two learners, newest first, without cancelled invitations.
