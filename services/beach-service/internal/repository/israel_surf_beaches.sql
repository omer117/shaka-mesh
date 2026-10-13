INSERT INTO beaches (id, name, latitude, longitude, location, description)
VALUES
    ('il-betzet', 'Betzet Beach', 33.0820, 35.1050, ST_SetSRID(ST_MakePoint(35.1050, 33.0820), 4326)::geography, 'Open northern beach break; exposed to Mediterranean winter swells.'),
    ('il-akhziv', 'Akhziv Beach', 33.0580, 35.1030, ST_SetSRID(ST_MakePoint(35.1030, 33.0580), 4326)::geography, 'Rocky northern coastline with exposed peaks when the swell is up.'),
    ('il-nahariya', 'Sokolov Beach, Nahariya', 33.0090, 35.0940, ST_SetSRID(ST_MakePoint(35.0940, 33.0090), 4326)::geography, 'Urban beach break on the Western Galilee coast.'),
    ('il-kiryat-yam', 'Kiryat Yam Beach', 32.8510, 35.0680, ST_SetSRID(ST_MakePoint(35.0680, 32.8510), 4326)::geography, 'Open Haifa Bay beach that can pick up winter swell.'),
    ('il-bat-galim', 'Bat Galim Beach, Haifa', 32.8290, 34.9710, ST_SetSRID(ST_MakePoint(34.9710, 32.8290), 4326)::geography, 'A well-known Haifa surf area with a defined point and reef setup.'),
    ('il-dado', 'Dado Beach, Haifa', 32.8060, 34.9610, ST_SetSRID(ST_MakePoint(34.9610, 32.8060), 4326)::geography, 'Open southern Haifa beach with sand-bottom peaks in suitable swell.'),
    ('il-atlit', 'Atlit Beach', 32.6900, 34.9410, ST_SetSRID(ST_MakePoint(34.9410, 32.6900), 4326)::geography, 'Exposed Carmel Coast beach break for regional swell days.'),
    ('il-sdot-yam', 'Caesarea Aqueduct Beach', 32.5010, 34.8910, ST_SetSRID(ST_MakePoint(34.8910, 32.5010), 4326)::geography, 'The Caesarea aqueduct coastline has rocky sections and exposed surf peaks.'),
    ('il-hadera-olga', 'Olga Beach, Hadera', 32.4590, 34.8750, ST_SetSRID(ST_MakePoint(34.8750, 32.4590), 4326)::geography, 'Hadera beach break with open exposure to west and northwest swell.'),
    ('il-netanya-sironit', 'Sironit Beach, Netanya', 32.3260, 34.8510, ST_SetSRID(ST_MakePoint(34.8510, 32.3260), 4326)::geography, 'Central Netanya beach with surfable sand-bottom peaks in swell.'),
    ('il-netanya-poleg', 'Poleg Beach, Netanya', 32.2690, 34.8400, ST_SetSRID(ST_MakePoint(34.8400, 32.2690), 4326)::geography, 'Southern Netanya beach break popular with local surfers.'),
    ('il-herzliya', 'Herzliya Beach', 32.1650, 34.7950, ST_SetSRID(ST_MakePoint(34.7950, 32.1650), 4326)::geography, 'Open central-coast beach with shifting sandbar peaks.'),
    ('il-tel-aviv-gordon', 'Gordon Beach, Tel Aviv', 32.0800, 34.7670, ST_SetSRID(ST_MakePoint(34.7670, 32.0800), 4326)::geography, 'Central Tel Aviv beach break with peaks along the beachfront.'),
    ('il-tel-aviv-frishman', 'Frishman Beach, Tel Aviv', 32.0750, 34.7670, ST_SetSRID(ST_MakePoint(34.7670, 32.0750), 4326)::geography, 'City beach break near the Tel Aviv promenade.'),
    ('il-jaffa-givat-aliya', 'Givat Aliya Beach, Jaffa', 32.0390, 34.7520, ST_SetSRID(ST_MakePoint(34.7520, 32.0390), 4326)::geography, 'Southern Tel Aviv coastline with exposed beach-break peaks.'),
    ('il-bat-yam', 'Tayo Beach, Bat Yam', 32.0130, 34.7430, ST_SetSRID(ST_MakePoint(34.7430, 32.0130), 4326)::geography, 'Bat Yam beach break that can work on Mediterranean swell.'),
    ('il-ashdod', 'Lido Beach, Ashdod', 31.8050, 34.6350, ST_SetSRID(ST_MakePoint(34.6350, 31.8050), 4326)::geography, 'Ashdod city beach with open exposure to west swells.'),
    ('il-ashkelon', 'Delilah Beach, Ashkelon', 31.6730, 34.5540, ST_SetSRID(ST_MakePoint(34.5540, 31.6730), 4326)::geography, 'Southern coastal beach break that picks up winter swell.'),
    ('il-zikim', 'Zikim Beach', 31.6110, 34.5130, ST_SetSRID(ST_MakePoint(34.5130, 31.6110), 4326)::geography, 'Open southern beach break near the Gaza border; check local access guidance.')
ON CONFLICT (id) DO NOTHING;
