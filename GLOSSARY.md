# Jak tam to słowo

An iPhone app for Russian speakers who learn Polish: its words and its grammar. The members of a group share one collection of cards, study the cards in both directions and race each other.

## Language

### People and groups

**Learner**: A signed-in user of the app. Each learner has one direction and their own study progress. The code calls it `users`. _Avoid_: account, for the person.

**Group**: The learners who share one collection of cards. A learner is in at most one group, and all members of a group are equal. The code calls it `groups`.

**Member**: A learner in a group.

**Invitation**: An email from a member with a single-use link that lets one person sign up or join the group. It is valid for 7 days. The code calls it `invitations`.

**Invitation token**: The random secret in the invitation link. A sign-up needs a valid, unused and unexpired token. The database stores only the hash of the token.

### Cards

**Card**: One item of the group's collection. Its kind is `translation` or `grammar`. The code calls it `cards`.

**Translation card**: A card with a Polish term and a Russian term, examples in both languages, a Russian meaning and alternative terms.

**Grammar card**: A card for one mistake from the learner's own text. The learner studies it as a gap sentence.

**Draft**: The AI's proposal for a new card. The learner checks it and can edit it before saving.

**Topic**: One short Polish label for the grammar topic of a grammar card, for example "przypadek" or "aspekt". The code calls it `topic`. _Avoid_: mistake type.

**Used form**: The wrong form exactly as the learner wrote it. The code calls it `used`.

**Right form**: The correct form for the learner's sentence. The code calls it `right`.

**Gap sentence**: A short Polish sentence with ___ in place of the answer. The code calls it `sentence`.

**Answer**: The words that fill the gap. The code calls it `answer`.

**Hint**: The text next to the gap sentence that makes the expected answer clear. When the mistake is about one word, it is the dictionary form in brackets, also when the learner wrote that form. The code calls it `hint`.

**Chips**: Short word facts and slot labels, for example "rodzaj męski" or "dopełniacz". The code calls them `chips`.

**Rule**: One or two sentences in simple Polish that explain the mistake. The code calls it `rule`.

**Examples**: Two or three correct Polish sentences that use the rule. The code calls them `examples`.

**Table**: An optional table, for example the endings of a case. Exactly one row is marked as the row for the card. The code calls it `table`.

### Study

**Direction**: Polish to Russian or Russian to Polish. Translation cards use the learner's direction. Grammar cards always ask for the Polish answer.

**Queue**: The group's cards that are due for the signed-in learner, in study order.

**Progress**: The correct and incorrect counts and times of one learner for one card. The code calls it `card_progress`.

**Review**: One graded answer of a learner to a card. The attempt id makes a review idempotent. The code calls it `reviews`.

**Grade**: The result of a review: correct, almost or again.

**Dictation**: An answer or a new word that the learner speaks. One clip holds one language. The text goes into the editable field, and the learner sends it.

### Races

**Race**: A contest of group members to the target score in one direction. A group has at most one open race. The code calls it `races`. _Avoid_: competition, match, game.

**Participant**: A member who is in a race. A score never goes below 0. The code calls it `race_participants`.

**Target score**: The score that wins a race. The member who starts the race picks it.

**Race direction**: The translation direction of a race. Only answers in this direction count.

**Race training**: Study inside the Zawody tab in the race direction. It is the only place where answers count for the race.

### Background work and consent

**Job**: An AI call that runs in the background while the app polls it. The code calls it `jobs`.

**AI consent**: The learner's permission to send text and audio to the AI services. The backend stores it as a time.
