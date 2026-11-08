# Jak tam to słowo

A vocabulary app for two learners of Polish and Russian. They share one collection of cards, study the cards in both directions and race each other in competitions.

## Language

### Cards

**Translation card**: A card with a Polish term and a Russian term, each with an example. The code calls this card kind `translation`.

**Grammar card**: A card that drills one form of one Polish word. It has one sentence with a gap, one answer and the grammar details. The code calls this card kind `grammar`.

**Word**: The dictionary form of the Polish word on a grammar card, as the learner typed it. The code stores it in the `pl` column.

**Used form**: The word or short phrase as the learner used it, for example "dwa piesy". The code calls it `used`.

**Right form**: The used form after the correction, for example "dwa psy". The check screen and Słówka show it. The code calls it `right`.

**Answer**: The text that fills the gap in the sentence, for example "psy". The learner types it in study. The code calls it `answer`.

**Word facts**: Short labels about the word itself, for example its part of speech and its gender. The app shows them as grey chips. The code calls them `facts`.

**Slot labels**: Short labels about the slot that was wrong, for example the case, the number and the ending. The app shows them as green chips and as the hint in Słówka. The code calls them `slot`.

**Mistake type**: One short label that names the error, for example "końcówka" or "przypadek". It is null when the used form is already correct. The code calls it `mistake`.

**Endings table**: The table of endings for the case or form of the answer. Rows with equal endings merge into one row, and exactly one row is marked as the row for the word. The code calls it `table`.

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
