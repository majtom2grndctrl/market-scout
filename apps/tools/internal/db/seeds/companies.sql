-- Canonical company seed file. Hand-edit for one-off additions;
-- cmd/onboard appends verified records from research sidecars.
-- Re-run safely: ON CONFLICT (ats, board_token) DO NOTHING.
-- See agent-context/lib/watchlist.md for context on adding companies
-- and the verification workflow.

INSERT INTO companies (name, ats, board_token, industry) VALUES
    -- Greenhouse
    ('Anthropic',  'greenhouse', 'anthropic',  'AI research & models'),
    ('Stripe',     'greenhouse', 'stripe',      'fintech'),
    ('Figma',      'greenhouse', 'figma',       'productivity & collaboration'),
    ('Scale AI',   'greenhouse', 'scaleai',     'AI infrastructure'),
    ('Glean',      'greenhouse', 'gleanwork',   'productivity & collaboration'),

    -- Ashby
    ('Cognition',  'ashby', 'cognition',  'dev tooling'),
    ('Harvey',     'ashby', 'harvey',     'legal tech'),
    ('ElevenLabs', 'ashby', 'elevenlabs', 'AI research & models'),
    ('Linear',     'ashby', 'linear',     'dev tooling'),

    -- Lever
    ('Mistral',    'lever', 'mistral',    'AI research & models'),

    -- Greenhouse (GeekWire batch, May 2026)
    ('Temporal',   'greenhouse', 'temporaltechnologies', 'dev tooling'),
    ('Gradial',    'greenhouse', 'gradial',               'sales & marketing tech'),

    -- Ashby (GeekWire batch, May 2026)
    ('Nooks',                  'ashby', 'nooks',      'sales & marketing tech'),
    ('Oumi',                   'ashby', 'oumi',       'AI research & models'),
    ('Ineffable Intelligence', 'ashby', 'ineffable',  'AI research & models'),

    -- Greenhouse (Built In Seattle / AI filter, May 2026)
    ('Runpod',  'greenhouse', 'runpod',  'AI infrastructure'),
    ('Phaidra', 'greenhouse', 'phaidra', 'robotics & hardware'),
    ('Textio',  'greenhouse', 'textio',  'HR & recruiting tech'),

    -- Ashby (Built In Seattle / AI filter, May 2026)
    ('Statsig',    'ashby', 'statsig',    'dev tooling'),
    ('Superhuman', 'ashby', 'superhuman', 'productivity & collaboration'),

    -- Greenhouse (Built In Seattle / full sweep, May 2026)
    ('CommerceIQ', 'greenhouse', 'commerceiq', 'sales & marketing tech'),
    ('super.AI',   'greenhouse', 'superai',    'AI infrastructure'),

    -- Lever (Built In Seattle / full sweep, May 2026)
    ('Spice AI',    'lever', 'spiceai',    'AI infrastructure'),
    ('Revefi',      'lever', 'revefi',     'data & analytics'),
    ('Avante',      'lever', 'avante',     'HR & recruiting tech'),
    ('Conversica',  'lever', 'conversica', 'sales & marketing tech'),

    -- Ashby (GeekWire funding tracker, May 2026)
    ('Cascade',  'ashby', 'cascade', 'HR & recruiting tech'),
    ('Humanly',  'ashby', 'humanly', 'HR & recruiting tech'),
    ('Union.ai', 'ashby', 'union',   'AI infrastructure'),
    ('Read AI',  'ashby', 'read-ai', 'productivity & collaboration'),
    ('QA Wolf',  'ashby', 'qawolf',  'dev tooling'),
    ('Depot',    'ashby', 'depot',   'dev tooling'),
    ('Casium',   'ashby', 'casium',  'legal tech'),

    -- Greenhouse (GeekWire funding tracker, May 2026)
    ('Chainguard',  'greenhouse', 'chainguard',  'security & identity'),
    ('Panthalassa', 'greenhouse', 'panthalassa', 'AI infrastructure'),
    ('Starcloud',   'greenhouse', 'starcloud',   'AI infrastructure'),

    -- Workable (GeekWire 200, May 2026)
    ('Seeq', 'workable', 'seeq', 'data & analytics')

ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Airtable', 'greenhouse', 'airtable', 'productivity & collaboration')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Truveta', 'greenhouse', 'truveta', 'healthtech'),
    ('Agility Robotics', 'greenhouse', 'agilityrobotics', 'robotics & hardware'),
    ('iSpot.tv', 'greenhouse', 'ispottv', 'sales & marketing tech'),
    ('Stoke Space', 'greenhouse', 'stokespacetechnologies', 'aerospace & defense'),
    ('Carbon Robotics', 'greenhouse', 'carbonrobotics', 'robotics & hardware'),
    ('Karat', 'greenhouse', 'karat', 'HR & recruiting tech'),
    ('Proprio', 'greenhouse', 'proprio', 'healthtech'),
    ('Customer.io', 'greenhouse', 'customerio', 'sales & marketing tech'),
    ('Place Technology', 'greenhouse', 'place', 'proptech & construction'),
    ('Pulumi', 'greenhouse', 'pulumicorporation', 'dev tooling'),
    ('Yoodli', 'greenhouse', 'yoodliinc', 'productivity & collaboration'),
    ('Stackline', 'greenhouse', 'stackline', 'commerce & marketplaces'),
    ('Group14', 'greenhouse', 'group14', 'climate & energy'),
    ('Snap! Raise', 'greenhouse', 'snapmobileinc', 'edtech'),
    ('Amperity', 'greenhouse', 'amperity', 'data & analytics'),
    ('Syndio', 'greenhouse', 'syndio', 'HR & recruiting tech'),
    ('Hyperproof', 'greenhouse', 'hyperproof', 'security & identity'),
    ('Boulder Care', 'greenhouse', 'bouldercare', 'healthtech'),
    ('Echodyne', 'greenhouse', 'echodynecorp', 'robotics & hardware'),
    ('LevelTen Energy', 'greenhouse', 'leveltenenergy', 'climate & energy'),
    ('Carbon Direct', 'greenhouse', 'carbondirect', 'climate & energy'),
    ('Archera.ai', 'greenhouse', 'archera', 'dev tooling'),
    ('Flexe', 'greenhouse', 'flexe', 'logistics'),
    ('Levanta', 'greenhouse', 'levanta', 'commerce & marketplaces'),
    ('TerraClear', 'greenhouse', 'terraclear', 'robotics & hardware'),
    ('Parse Biosciences', 'greenhouse', 'parsebiosciences', 'biotech'),
    ('Aspect Biosystems', 'greenhouse', 'aspectbiosystems', 'biotech'),
    ('Lumen Bioscience', 'greenhouse', 'lumenbioscience', 'biotech'),
    ('Digs', 'greenhouse', 'digs', 'proptech & construction'),
    ('Upbound', 'greenhouse', 'upbound', 'dev tooling'),
    ('Recurrent', 'greenhouse', 'recurrent', 'climate & energy'),
    ('Tune Therapeutics', 'greenhouse', 'tunetherapeutics', 'biotech'),
    ('Corelight', 'greenhouse', 'corelight', 'security & identity'),
    ('Attunely', 'greenhouse', 'attunely', 'fintech'),
    ('Legion', 'greenhouse', 'legion', 'HR & recruiting tech'),
    ('MediaAlpha', 'greenhouse', 'mediaalpha', 'sales & marketing tech'),
    ('Xealth', 'greenhouse', 'xealth', 'healthtech'),
    ('Tenable', 'greenhouse', 'tenableinc', 'security & identity'),
    ('OfferUp', 'greenhouse', 'offerup', 'commerce & marketplaces')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Brinc', 'ashby', 'brinc', 'aerospace & defense'),
    ('MotherDuck', 'ashby', 'MotherDuck', 'data & analytics'),
    ('Klue', 'ashby', 'klue', 'sales & marketing tech'),
    ('Certn', 'ashby', 'certn', 'HR & recruiting tech'),
    ('Vibe', 'ashby', 'vibe', 'sales & marketing tech'),
    ('Eigen Labs', 'ashby', 'eigen-labs', 'crypto'),
    ('Common Room', 'ashby', 'commonroom', 'sales & marketing tech'),
    ('Atlas Health', 'ashby', 'atlas', 'healthtech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Lever (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Outreach', 'lever', 'outreach', 'sales & marketing tech'),
    ('Omnidian', 'lever', 'omnidian', 'climate & energy'),
    ('Outpace Bio', 'lever', 'outpacebio', 'biotech'),
    ('Viome', 'lever', 'viome', 'healthtech'),
    ('DexCare', 'lever', 'dexcarehealth', 'healthtech'),
    ('Oleria', 'lever', 'oleria-security', 'security & identity'),
    ('Mast Reforestation', 'lever', 'MastReforestation', 'climate & energy'),
    ('Educative', 'lever', 'educative', 'edtech'),
    ('Sanctuary AI', 'lever', 'sanctuary', 'robotics & hardware'),
    ('SkyPoint Cloud', 'lever', 'skypointcloud', 'data & analytics'),
    ('Aigen', 'lever', 'aigen', 'climate & energy'),
    ('Lumotive', 'lever', 'lumotive', 'robotics & hardware'),
    ('Highspot', 'lever', 'highspot', 'sales & marketing tech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workable (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Starfish Space', 'workable', 'starfish-space-1', 'aerospace & defense'),
    ('Vouched', 'workable', 'vouched', 'security & identity'),
    ('Likewise', 'workable', 'likewise', 'commerce & marketplaces'),
    ('Discovery Health MD', 'workable', 'discovery-health-md', 'healthtech'),
    ('Banzai', 'workable', 'banzai', 'sales & marketing tech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workday (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Remitly', 'workday', 'remitly.wd5.myworkdayjobs.com/Remitly_Careers', 'fintech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Helion',   'ashby', 'helion',   'climate & energy'),
    ('Hiya',     'ashby', 'hiya',     'security & identity'),
    ('Qumulo',   'ashby', 'qumulo',   'dev tooling'),
    ('Polly',    'ashby', 'polly',    'fintech'),
    ('LiveKit',  'ashby', 'livekit',  'dev tooling')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workday (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Tinuiti', 'workday', 'tinuiti.wd12.myworkdayjobs.com/Tinuiti', 'sales & marketing tech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Lever (May 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Ridwell', 'lever', 'Ridwell', 'climate & energy')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (fetch list sync, July 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('XBOW', 'ashby', 'xbowcareers', 'Autonomous cybersecurity / AI pentesting'),
    ('OpenAI', 'ashby', 'openai', 'AI research'),
    ('Tin Can', 'ashby', 'tin-can', 'Consumer Electronics'),
    ('KamiwazaAI', 'ashby', 'kamiwaza', 'AI')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (fetch list sync, July 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Dropzone AI', 'greenhouse', 'dropzoneai', 'Computer & Network Security'),
    ('Contentstack', 'greenhouse', 'contentstack', NULL)
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workday (fetch list sync, July 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Sprinklr', 'workday', 'sprinklr.wd1.myworkdayjobs.com/careers', NULL)
ON CONFLICT (ats, board_token) DO NOTHING;

-- Gem (August 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Supio', 'gem', 'supio', 'legal tech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (Built In Seattle discovery run, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Coinme', 'greenhouse', 'coinme', 'Crypto / Fintech'),
    ('Databricks', 'greenhouse', 'databricks', 'Data & AI Platform'),
    ('DigitalOcean', 'greenhouse', 'digitalocean98', 'Cloud Infrastructure'),
    ('Dscout', 'greenhouse', 'dscout', 'UX Research Software'),
    ('ExtraHop', 'greenhouse', 'extrahopnetworks', 'Computer & Network Security'),
    ('Huntress', 'greenhouse', 'huntress', 'Computer & Network Security'),
    ('Impinj', 'greenhouse', 'impinjexternal', 'Semiconductors / RAIN RFID'),
    ('Nintex', 'greenhouse', 'nintex', 'Software Development'),
    ('Placements.io', 'greenhouse', 'placementsio', 'Ad Revenue Management Software'),
    ('SeekOut', 'greenhouse', 'seekout', 'HR Tech'),
    ('Zenoti', 'greenhouse', 'zenoti', 'Software Development')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (Built In Seattle discovery run, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Augmodo', 'ashby', 'augmodo', 'Retail AI / Spatial Computing'),
    ('Orchard Robotics', 'ashby', 'orchard', 'Agricultural Robotics'),
    ('Payscale', 'ashby', 'payscale', 'HR Tech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Lever (Built In Seattle discovery run, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Artera', 'lever', 'artera-2', 'Healthtech'),
    ('Teikametrics', 'lever', 'teikametrics', 'AI retail / advertising'),
    ('Zoox', 'lever', 'zoox', 'Autonomous Vehicles')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (Built In Seattle discovery run, batch 2, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Cognitiv', 'greenhouse', 'cognitiv', 'AI Advertising'),
    ('Mixpanel', 'greenhouse', 'mixpanel', 'Product Analytics'),
    ('MORSE Corp', 'greenhouse', 'morsecorp', 'Defense Tech / AI'),
    ('NewsBreak', 'greenhouse', 'newsbreak', 'Local News / Consumer App'),
    ('Pallet', 'greenhouse', 'pallet', 'Logistics AI'),
    ('SingleStore', 'greenhouse', 'singlestore', 'Database / Data Platform'),
    ('TaxBit', 'greenhouse', 'taxbit', 'Crypto Tax / Fintech'),
    ('Vannevar Labs', 'greenhouse', 'vannevarlabs', 'Defense Tech / AI')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (Built In Seattle discovery run, batch 2, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Envoy', 'ashby', 'envoy', 'Workplace Software'),
    ('Monte Carlo', 'ashby', 'montecarlodata', 'Data Observability'),
    ('Peek', 'ashby', 'peek', 'Experiences Booking Software'),
    ('SentiLink', 'ashby', 'sentilink', 'Fraud Prevention / Fintech')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Lever (Built In Seattle discovery run, batch 2, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Duranta', 'lever', 'getduranta', 'Landscaping Software'),
    ('Lucidworks', 'lever', 'lucidworks', 'Search / AI Platform')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Gem (Built In Seattle discovery run, batch 2, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Goodbill', 'gem', 'goodbill', 'Healthcare Billing'),
    ('Retool', 'gem', 'retool', 'Internal Tools / Developer Platform')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (Built In Seattle discovery run, batch 3, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('AppViewX', 'greenhouse', 'appviewx', 'Certificate Lifecycle / Security'),
    ('Axon', 'greenhouse', 'axon', 'Public Safety Technology'),
    ('Carta', 'greenhouse', 'carta', 'Cap Table / Fintech'),
    ('CoreWeave', 'greenhouse', 'coreweave', 'AI Cloud Infrastructure'),
    ('DAT Freight & Analytics', 'greenhouse', 'datsolutions', 'Freight Data / Logistics'),
    ('Duolingo', 'greenhouse', 'duolingo', 'EdTech'),
    ('Motivity', 'greenhouse', 'motivity', 'ABA Therapy Software / Healthtech'),
    ('Pushpay', 'greenhouse', 'pushpay', 'Church Engagement / Payments'),
    ('Tanium', 'greenhouse', 'tanium', 'Endpoint Security')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (Built In Seattle discovery run, batch 3, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Grow Therapy', 'ashby', 'grow-therapy', 'Mental Health / Healthtech'),
    ('Headway', 'ashby', 'headway', 'Mental Health / Healthtech'),
    ('LILT', 'ashby', 'lilt-corporate', 'AI Translation'),
    ('Mural', 'ashby', 'mural', 'Visual Collaboration'),
    ('Runway', 'ashby', 'runway-ml', 'Generative AI / Video'),
    ('Summation', 'ashby', 'summation', 'AI Business Planning')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Lever (Built In Seattle discovery run, batch 3, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Corbalt', 'lever', 'corbalt', 'GovTech / Infrastructure'),
    ('Magnify', 'lever', 'magnify', 'Customer Experience Software')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workday (Built In Seattle discovery run, batch 3, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('UserTesting', 'workday', 'usertesting.wd12.myworkdayjobs.com/UserTesting', 'UX Research Software')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (Built In Seattle discovery run, batch 4, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Cloudflare', 'greenhouse', 'cloudflare', 'Cloud Infrastructure / Security'),
    ('Flexport', 'greenhouse', 'flexport', 'Freight / Logistics'),
    ('impact.com', 'greenhouse', 'impact', 'Partnership Management / Martech'),
    ('MCG Health', 'greenhouse', 'mcghealth', 'Healthcare Guidelines / Healthtech'),
    ('SoFi', 'greenhouse', 'sofi', 'Fintech'),
    ('Veeam', 'greenhouse', 'veeamsoftware', 'Data Protection / Backup'),
    ('Zscaler', 'greenhouse', 'zscaler', 'Cloud Security')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (Built In Seattle discovery run, batch 4, September 2026)
-- Token stored lowercase: the careers page links jobs.ashbyhq.com/Spoton, and
-- Ashby tokens normalize to lowercase.
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('SpotOn', 'ashby', 'spoton', 'Restaurant / Retail Payments')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workday (Built In Seattle discovery run, batch 4, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('CrowdStrike', 'workday', 'crowdstrike.wd5.myworkdayjobs.com/crowdstrikecareers', 'Endpoint Security'),
    ('F5', 'workday', 'ffive.wd5.myworkdayjobs.com/f5jobs', 'Application Delivery / Security'),
    ('Porch Group', 'workday', 'porch.wd1.myworkdayjobs.com/careers', 'Home Services / Insurtech'),
    ('Sonos', 'workday', 'sonos.wd1.myworkdayjobs.com/Sonos', 'Consumer Audio'),
    ('Unity', 'workday', 'unitytech.wd1.myworkdayjobs.com/Unity', 'Game Engine / Developer Tools')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (Built In Seattle discovery run, batch 5, September 2026)
-- Block covers the Square and Cash App entries in the same source: both are
-- Block brands hiring through one Greenhouse board.
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Block', 'greenhouse', 'block', 'Fintech / Payments'),
    ('Metropolis Technologies', 'greenhouse', 'metropolis', 'Computer Vision / Parking')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (Built In Seattle discovery run, batch 5, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('JAMS Software', 'ashby', 'jamssoftware', 'Job Scheduling / IT Orchestration'),
    ('Luxor Technology', 'ashby', 'luxor', 'Bitcoin Mining Software'),
    ('Rowan Digital Infrastructure', 'ashby', 'rowan', 'Data Center Infrastructure')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workable (Built In Seattle discovery run, batch 5, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Squiz', 'workable', 'squiz', 'Digital Experience Platform / CMS'),
    ('Vix Technology', 'workable', 'vix-technology', 'Transit Ticketing Technology')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workday (Built In Seattle discovery run, batch 5, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Astound Broadband', 'workday', 'astound.wd108.myworkdayjobs.com/Astound_Careers', 'Telecommunications'),
    ('CyrusOne', 'workday', 'cyrusone.wd1.myworkdayjobs.com/CyrusOneCareerPortal', 'Data Center Infrastructure'),
    ('Expedia Group', 'workday', 'expedia.wd108.myworkdayjobs.com/search', 'Travel Technology'),
    ('NVIDIA', 'workday', 'nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite', 'Semiconductors / AI')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Greenhouse (Built In Seattle discovery run, batch 6, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('CannonDesign', 'greenhouse', 'cannondesign', 'Architecture & Engineering Design'),
    ('DLR Group', 'greenhouse', 'dlrgroup', 'Architecture & Engineering Design')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Ashby (Built In Seattle discovery run, batch 6, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Aurelian', 'ashby', 'aurelian', 'Public Safety AI / Emergency Communications'),
    ('Pariveda Solutions', 'ashby', 'pariveda', 'Technology Consulting')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Lever (Built In Seattle discovery run, batch 6, September 2026)
-- Arc'teryx's Lever board is keyed on a domain-style token rather than a bare
-- slug. Lever tokens are case-sensitive and stored verbatim, so the dot is not
-- a typo and must not be normalized away.
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Arc''teryx', 'lever', 'arcteryx.com', 'Outdoor Apparel & Equipment'),
    ('Reply', 'lever', 'reply', 'IT Consulting / Systems Integration'),
    ('Woven by Toyota', 'lever', 'woven-by-toyota', 'Automotive Software / Autonomous Systems')
ON CONFLICT (ats, board_token) DO NOTHING;

-- Workday (Built In Seattle discovery run, batch 6, September 2026)
INSERT INTO companies (name, ats, board_token, industry) VALUES
    ('Russell Investments', 'workday', 'russell.wd5.myworkdayjobs.com/russellinvestments', 'Asset Management / Fintech')
ON CONFLICT (ats, board_token) DO NOTHING;
