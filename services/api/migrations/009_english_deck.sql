-- Migration: 009_english_deck
-- Adds an official CEFR B2 Academic Word List starter deck for English
-- learners (intermediate/advanced). Mirrors the JLPT/HSK/TOPIK seed pattern.

WITH en_deck AS (
    INSERT INTO decks (language_code, name, description, is_public, is_official, tags)
    VALUES (
        'en',
        'CEFR B2 Academic Word List',
        'High-utility academic vocabulary for intermediate-to-advanced English learners',
        TRUE, TRUE,
        ARRAY['cefr', 'b2', 'academic', 'intermediate']
    )
    RETURNING id
),
en_cards(front, back) AS (
    VALUES
        ('analyze',       'to examine in detail; to break down into components'),
        ('approach',      'a method or way of dealing with something'),
        ('benefit',       'an advantage or profit gained from something'),
        ('concept',       'an abstract idea or general notion'),
        ('consist',       'to be made up of (with "of")'),
        ('constitute',    'to form or compose'),
        ('context',       'the circumstances surrounding an event'),
        ('contract',      'a binding agreement between parties'),
        ('contribute',    'to give in order to help achieve something'),
        ('create',        'to bring something into existence'),
        ('data',          'facts and statistics collected for reference'),
        ('define',        'to state the precise meaning of'),
        ('derive',        'to obtain from a source'),
        ('distribute',    'to give or hand out among recipients'),
        ('economy',       'the wealth and resources of a country'),
        ('environment',   'the surroundings or conditions in which one lives'),
        ('establish',     'to set up on a firm basis'),
        ('estimate',      'to roughly calculate the value of'),
        ('evident',       'clearly seen or understood'),
        ('factor',        'a circumstance that contributes to a result'),
        ('finance',       'the management of large sums of money'),
        ('formula',       'a rule or fact expressed in symbols'),
        ('function',      'an activity that is natural or intended'),
        ('identify',      'to recognize or establish what someone is'),
        ('income',        'money received for work or investments'),
        ('indicate',      'to point out or show'),
        ('individual',    'a single human being, distinct from a group'),
        ('interpret',     'to explain the meaning of'),
        ('involve',       'to include as a necessary part'),
        ('issue',         'an important topic for debate'),
        ('labor',         'work, especially physical effort'),
        ('legal',         'relating to the law'),
        ('legislate',     'to make laws'),
        ('major',         'important or significant'),
        ('method',        'a procedure for accomplishing something'),
        ('occur',         'to happen or take place'),
        ('percent',       'one part in every hundred'),
        ('period',        'a length or portion of time'),
        ('policy',        'a course of action adopted by an organization'),
        ('principle',     'a fundamental truth or rule'),
        ('proceed',       'to begin or continue a course of action'),
        ('process',       'a series of actions leading to an end'),
        ('require',       'to need for a purpose'),
        ('research',      'systematic investigation into a subject'),
        ('respond',       'to react or reply'),
        ('role',          'a function or part played'),
        ('section',       'a distinct part of something'),
        ('sector',        'a distinct part of an economy or society'),
        ('significant',   'sufficiently great or important'),
        ('similar',       'resembling but not identical'),
        ('source',        'a place from which something comes'),
        ('specific',      'clearly defined or particular'),
        ('structure',     'the arrangement of related parts'),
        ('theory',        'a system of ideas explaining something'),
        ('vary',          'to differ in quantity or quality'),
        ('acquire',       'to gain possession of'),
        ('adequate',      'satisfactory or acceptable'),
        ('assume',        'to suppose to be the case without proof'),
        ('attribute',     'a quality or feature regarded as characteristic'),
        ('communicate',   'to share or exchange information'),
        ('contradict',    'to deny the truth of by asserting the opposite'),
        ('controversy',   'prolonged public disagreement'),
        ('demonstrate',   'to clearly show the existence or truth of'),
        ('despite',       'without being affected by'),
        ('emphasize',     'to give special importance to'),
        ('evaluate',      'to form an idea of the value of'),
        ('hypothesis',    'a proposed explanation for further investigation'),
        ('inevitable',    'certain to happen, unavoidable'),
        ('influence',     'the capacity to have an effect on something'),
        ('phenomenon',    'a fact or situation observed to exist'),
        ('predominant',   'present as the strongest element'),
        ('regulate',      'to control by means of rules'),
        ('reluctant',     'unwilling or hesitant'),
        ('reveal',        'to make something previously secret known'),
        ('subsequent',    'coming after something in time'),
        ('substantial',   'of considerable importance, size, or worth'),
        ('sufficient',    'enough; adequate'),
        ('synthesize',    'to combine into a coherent whole'),
        ('undertake',     'to commit oneself to a task'),
        ('utilize',       'to make practical use of')
)
INSERT INTO cards (user_id, deck_id, language_code, front_text, back_text, fsrs_state)
SELECT
    '00000000-0000-0000-0000-000000000000'::uuid,
    en_deck.id,
    'en',
    en_cards.front,
    en_cards.back,
    'new'
FROM en_deck, en_cards
ON CONFLICT DO NOTHING;

UPDATE decks SET card_count = (
    SELECT COUNT(*) FROM cards WHERE deck_id = decks.id AND deleted_at IS NULL
)
WHERE name = 'CEFR B2 Academic Word List';
