package search

import (
	"strconv"
	"strings"

	"github.com/BurntSushi/ty/fun"

	"github.com/BurntSushi/goim/imdb"
)

// Commands represents all available search directives that are available
// in a search query string.
var Commands []Command

// Command represents a single search directive available in a search query
// string. Each command has a canonical name, a list of possibly empty
// synonyms and a brief description describing what the directive does.
type Command struct {
	Name        string
	Synonyms    []string
	Description string
}

// A command is a directive included in a string representation of a search.
// They are of the form '{name:value}', where 'value' is interpreted specially
// depending upon the command.
//
// A command may also have synonyms. For example '{season:1-5}' can also be
// expressed more tersely as '{s:1-5}'.
type command struct {
	name        string
	synonyms    []string
	description string
	add         func(s *Searcher, value string) error
}

func addRange(v string, max int, add func(mn, mx int) *Searcher) error {
	if mn, mx, err := intRange(v, 0, max); err != nil {
		return err
	} else {
		add(mn, mx)
		return nil
	}
}

func addSub(s *Searcher, name, v string, add func(*Searcher) *Searcher) error {
	sub, err := s.subSearcher(name, v)
	if err != nil {
		return err
	}
	add(sub)
	return nil
}

var (
	// commands corresponds to the single point of truth about all possible
	// search commands. There is exactly one 'command' value for each
	// logical command directive.
	commands []command

	// allCommands represents the same information in commands, except it's
	// represented as a map where keys are command names. (Synonyms are
	// included in the keys.)
	allCommands = map[string]command{}
)

func init() {
	commands = []command{
		{
			"movie", nil,
			"Restricts results to only include movies. Note that this may " +
				"be combined with other entity types to form a disjunction.",
			func(s *Searcher, v string) error {
				s.Entity(imdb.EntityMovie)
				return nil
			},
		},
		{
			"tvshow", nil,
			"Restricts results to only include TV shows. Note that this may " +
				"be combined with other entity types to form a disjunction.",
			func(s *Searcher, v string) error {
				s.Entity(imdb.EntityTvshow)
				return nil
			},
		},
		{
			"episode", nil,
			"Restricts results to only include episodes. Note that this may " +
				"be combined with other entity types to form a disjunction.",
			func(s *Searcher, v string) error {
				s.Entity(imdb.EntityEpisode)
				return nil
			},
		},
		{
			"actor", nil,
			"Restricts results to only include actors. Note that this may " +
				"be combined with other entity types to form a disjunction.",
			func(s *Searcher, v string) error {
				s.Entity(imdb.EntityActor)
				return nil
			},
		},
		{
			"credits", nil,
			"A sub-search for media entities that restricts results to " +
				"only actors media item returned from this sub-search.",
			func(s *Searcher, v string) error {
				return addSub(s, "credits", v, s.Credits)
			},
		},
		{
			"cast", nil,
			"A sub-search for cast entities that restricts results to " +
				"only media entities in which the cast member appeared.",
			func(s *Searcher, v string) error {
				return addSub(s, "cast", v, s.Cast)
			},
		},
		{
			"show", nil,
			"A sub-search for TV shows that restricts results to " +
				"only episodes in the TV show.",
			func(s *Searcher, v string) error {
				return addSub(s, "show", v, s.Tvshow)
			},
		},
		{
			"debug", nil,
			"When enabled, the SQL queries used in the search will be logged " +
				"to stderr.",
			func(s *Searcher, v string) error {
				s.debug = true
				return nil
			},
		},
		{
			"id", []string{"atom"},
			"Precisely selects a single identity with the atom identifier " +
				"given. e.g., {id:123} returns the entity with id 123." +
				"Note that one SHOULD NOT rely on any specific atom " +
				"identifier to always correspond to a specific entity, since " +
				"identifiers can (sadly) change when updating your database.",
			func(s *Searcher, v string) error {
				n, err := strconv.Atoi(v)
				if err != nil {
					return ef("Invalid integer '%s' for atom id: %s", v, err)
				}
				s.Atom(imdb.Atom(n))
				return nil
			},
		},
		{
			"years", []string{"year"},
			"Only show search results for the year or years specified. " +
				"e.g., {1990-1999} only shows movies in the 90s.",
			func(s *Searcher, v string) error {
				return addRange(v, maxYear, s.Years)
			},
		},
		{
			"rank", nil,
			"Only show search results with the rank or ranks specified. " +
				"e.g., {70-} only shows entities with a rank of 70 or " +
				"better. Ranks are on a scale of 0 to 100, where 100 is the " +
				"best.",
			func(s *Searcher, v string) error {
				return addRange(v, maxRank, s.Ranks)
			},
		},
		{
			"votes", nil,
			"Only show search results with ranks that have the vote count " +
				"specified. e.g., {10000-} only shows entities with a rank " +
				"that has 10,000 or more votes.",
			func(s *Searcher, v string) error {
				return addRange(v, maxVotes, s.Votes)
			},
		},
		{
			"billed", []string{"billing"},
			"Only show search results with credits with the billing position " +
				"specified. e.g., {1-5} only shows movies where the actor " +
				"was in the top 5 billing order (or only shows actors of a " +
				"movie in the top 5 billing positions).",
			func(s *Searcher, v string) error {
				return addRange(v, maxBilled, s.Billed)
			},
		},
		{
			"seasons", []string{"s"},
			"Only show search results for the season or seasons specified. " +
				"e.g., {seasons:1} only shows episodes from the first season " +
				"of a TV show. Note that this only filters episodes---movies " +
				"and TV shows are still returned otherwise.",
			func(s *Searcher, v string) error {
				return addRange(v, maxSeason, s.Seasons)
			},
		},
		{
			"episodes", []string{"e"},
			"Only show search results for the season or seasons specified. " +
				"e.g., {episodes:1-5} only shows the first five episodes of " +
				"a of a season. Note that this only filters " +
				"episodes---movies and TV shows are still returned otherwise.",
			func(s *Searcher, v string) error {
				return addRange(v, maxEpisode, s.Episodes)
			},
		},
		{
			"notv", nil,
			"Removes 'made for TV' movies from the search results.",
			func(s *Searcher, v string) error {
				s.NoTvMovies()
				return nil
			},
		},
		{
			"novideo", nil,
			"Removes 'made for video' movies from the search results.",
			func(s *Searcher, v string) error {
				s.NoVideoMovies()
				return nil
			},
		},
		{
			"limit", nil,
			"Specifies a limit on the total number of search results returned.",
			func(s *Searcher, v string) error {
				n, err := strconv.Atoi(v)
				if err != nil {
					return ef("Invalid integer '%s' for limit: %s", v, err)
				}
				s.Limit(int(n))
				return nil
			},
		},
		{
			"sort", nil,
			"Sorts the search results according to the field given. It may " +
				"be specified multiple times for more specific sorting. Note " +
				"that this doesn't really work with fuzzy searching, since " +
				"results are always sorted by their similarity with the " +
				"query in a fuzzy search. e.g., {sort:episode desc} sorts " +
				"episode in descending (biggest to smallest) order.",
			func(s *Searcher, v string) error {
				fields := strings.Fields(v)
				if len(fields) == 0 || len(fields) > 2 {
					return ef("Invalid sort format: '%s'", v)
				}

				var order string
				if len(fields) > 1 {
					order = fields[1]
				} else {
					order = defaultOrders[fields[0]]
					if len(order) == 0 {
						order = "asc"
					}
				}
				s.Sort(fields[0], order)
				return nil
			},
		},
	}

	// Add synonyms of commands to the map of commands.
	for _, cmd := range commands {
		allCommands[cmd.name] = cmd
		for _, synonym := range cmd.synonyms {
			allCommands[synonym] = cmd
		}
		Commands = append(Commands, Command{
			Name:        cmd.name,
			Synonyms:    cmd.synonyms,
			Description: cmd.description,
		})
	}
	fun.Sort(func(c1, c2 Command) bool { return c1.Name < c2.Name }, Commands)
}