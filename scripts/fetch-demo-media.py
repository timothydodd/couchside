#!/usr/bin/env python3
"""Download free-to-show demo media for a Couchside demo server.

Everything here is either Creative Commons Attribution (the Blender open
movies: credit "Blender Foundation | www.blender.org" and keep the films' own
credits) or public domain in the US (the Internet Archive items, all marked
public domain there). Public domain status is US-only; restorations and new
scores can carry their own copyright, so the items below are plain prints.

    python fetch-demo-media.py D:\\couchside-demo            # movies, TV, commercials
    python fetch-demo-media.py /mnt/media/demo --only movies tv
    python fetch-demo-media.py /mnt/media/demo --only cartoons  # tens of GB; not in the default run
    python fetch-demo-media.py /mnt/media/demo --list         # sizes, no download

Lays files out the way Couchside (and Plex) expect:

    <root>/Movies/Big Buck Bunny (2008)/Big Buck Bunny (2008).mp4
    <root>/TV/The Beverly Hillbillies (1962)/Season 01/The Beverly Hillbillies - S01E01 - The Clampetts Strike Oil.mp4
    <root>/Cartoons/Falling Hare (1943)/Falling Hare (1943).mkv
    <root>/Commercials/Cheerios V-8 (1960).mp4

Add Movies, TV and Cartoons (as a movie library) as libraries. Commercials is filler for virtual channels,
not a library. Re-running skips finished files and resumes partial ones.
Python 3.9+, standard library only.
"""

import argparse
import json
import os
import re
import shutil
import sys
import time
import urllib.error
import urllib.request
import zipfile

UA = "Mozilla/5.0 (couchside demo fetcher; +https://couchside.app)"
NLUUG = "https://ftp.nluug.nl/pub/graphics/blender/demo/movies"  # official Blender mirror

# --- what to fetch --------------------------------------------------------------
# Each movie: (folder/file name, source). A source is ("url", url) for a direct
# file, ("zip", url) for a mirror file that's zipped, or ("ia", item, file) for
# an Internet Archive file.

MOVIES = [
    ("Big Buck Bunny (2008)", ("zip", f"{NLUUG}/BBB/bbb_sunflower_1080p_30fps_normal.mp4.zip")),
    ("Sintel (2010)", ("url", f"{NLUUG}/Sintel.2010.1080p.mkv")),  # subtitles are inside the mkv
    ("Tears of Steel (2012)", ("zip", f"{NLUUG}/ToS/tears_of_steel_1080p.mov.zip")),
    ("Night of the Living Dead (1968)", ("ia", "Night.Of.The.Living.Dead_1080p", "NightOfTheLivingDead_1080p.mp4")),
    ("His Girl Friday (1940)", ("ia", "his_girl_friday", "his_girl_friday.mp4")),
    ("The General (1926)", ("ia", "The_General_Buster_Keaton", "The_General.mp4")),
    # Low quality on purpose: shows the SD badge and the Optimize/transcode paths.
    ("Nosferatu (1922)", ("ia", "Nosferatu_most_complete_version_93_mins.", "Nosferatu_1922_Symphony_of_Horror_512kb.mp4")),
]

# Sidecar subtitles: (movie folder, language, url). Saved as "<Movie>.<lang>.srt".
SUBTITLES = [
    ("Tears of Steel (2012)", lang, f"{NLUUG}/ToS/subtitles/TOS-{src}.srt")
    for lang, src in [("en", "en"), ("es", "es"), ("de", "de"), ("fr", "fr-orig"), ("it", "it"), ("nl", "nl"), ("ja", "JP"), ("ru", "ru")]
]

# TV: the public-domain Beverly Hillbillies episodes (season 1; the copyrights
# weren't renewed). Episodes are found by their sNNeNN file names.
TV = [
    ("The Beverly Hillbillies (1962)", "The Beverly Hillbillies", "bevhill-s01e01-36"),
]

# Filler for virtual channels: public-domain 1950s/60s TV commercials from the
# Prelinger Archives.
COMMERCIALS = [
    ("Cheerios V-8 (1960)", "Cheerios1960"),
    ("A Wonderful New World of Fords (1960)", "Wonderfu1960"),
    ("1955 Chevrolet Screen Ads (1955)", "1955Chev1955"),
    ("Classic Television Commercials Part 1 (1948)", "ClassicT1948"),
    ("Classic Television Commercials Part 2 (1948)", "ClassicT1948_2"),
    ("Classic Television Commercials Part 3 (1948)", "ClassicT1948_3"),
    ("Television Commercials 1950s-1960s (1960)", "Televisi1960"),
]

# Cartoons: the Warner Bros. shorts that are public domain in the US, from the
# Looney Tunes Wiki's list. Most lost their copyright when it wasn't renewed;
# 1929-30 ones have since expired. Each is (title, year, Internet Archive item,
# file), picked as the best copy there by resolution, then bitrate. Left out:
# colorized and "redrawn" versions (still under copyright), copies labelled as
# DVD or Blu-ray rips, captures of TV broadcasts and of video sites, foreign
# dubs, "Bosko's Dizzy Date" (only a re-edit is public domain), and the shorts
# built on racial or wartime caricature (the "Censored Eleven" ones among
# them): this library is shown to app-store reviewers. Not on the Archive: the unreleased Snafus "Going Home"
# (only as a Blu-ray rip) and "Secrets of the Caribbean".
CARTOONS = [
    # Looney Tunes and Merrie Melodies
    ('Bosko the Talk-Ink Kid', 1929, 'bosko-the-talk-ink-kid-1929_202406', 'Bosko the Talk-Ink Kid (1929).mp4'),
    ("Sinkin' in the Bathtub", 1930, '0001-sinkin-in-the-bathtub-web', "0001 - Sinkin' in the Bathtub (Web).mp4"),
    ('Congo Jazz', 1930, 'looney-tunes-congo-jazz', 'Looney Tunes - Congo Jazz.mp4'),
    ('Hold Anything', 1930, 'hold-anything_1930', 'hold-anything_1930.mp4'),
    ('The Booze Hangs High', 1930, 'TheBoozeHangsHigh1930HughHarmanRudolfIsing', 'The Booze Hangs High 1930 Harman & Ising.avi'),
    ('Box Car Blues', 1930, 'BoxCarBlues1930HughHarmanRudolfIsing', 'Box Car Blues 1930 Hugh Harman, Rudolf Ising.avi'),
    ('Big Man from the North', 1930, 'BigManFromTheNorthCartoon', 'BigManFromTheNorthCartoon.mp4'),
    ("Ain't Nature Grand!", 1930, 'AintNatureGrand1931HughHarmanRudolfIsing', "Ain't Nature Grand 1931 Hugh Harman, Rudolf Ising CC.avi"),
    ("Ups 'n Downs", 1931, 'UpsNDowns1931HughHarmanRudolfIsing', "Ups 'N Downs 1931 Hugh Harman, Rudolf Ising.avi"),
    ('Dumb Patrol', 1931, 'the-dumb-patrol-1931', 'The Dumb Patrol (1931).mp4'),
    ('Yodeling Yokels', 1931, 'YodelingYokels1931HughHarmanRudolfIsing', 'Yodeling Yokels 1931 Hugh Harman, Rudolf Ising.avi'),
    ("Bosko's Holiday", 1931, 'BoskosHoliday1931HughHarmanRudolfIsing', "Bosko's Holiday 1931 Hugh Harman, Rudolf Ising.avi"),
    ("The Tree's Knees", 1931, 'TheTreesKnees1931HughHarmanRudolfIsing', "The Tree's Knees 1931 Hugh Harman, Rudolf Ising.avi"),
    ('Lady, Play Your Mandolin!', 1931, 'ssVid.net--Merrie-Melodies-Lady-Play-Your-Mandolin-HD-1931_1080p', 'ssVid.net--Merrie-Melodies-Lady-Play-Your-Mandolin-HD-1931_1080p.mp4'),
    ('Smile, Darn Ya, Smile!', 1931, 'smile-darn-ya-smile-1931-restored', 'S01E001_Smile, Darn Ya, Smile.mp4'),
    ('Bosko Shipwrecked!', 1931, 'BoskoShipwrecked1931HughHarmanRudolfIsing', 'Bosko Shipwrecked 1931 Hugh Harman, Rudolf Ising.avi'),
    ('One More Time', 1931, 'OneMoreTime1931HughHarmanRudolfIsing', 'One More Time 1931 Hugh Harman, Rudolf Ising.avi'),
    ('Bosko the Doughboy', 1931, 'BoskoTheDoughboy1931HughHarmanRudolfIsing', 'Bosko The Doughboy 1931 Hugh Harman, Rudolf Ising.avi'),
    ("You Don't Know What You're Doin'!", 1931, 'YouDontKnowWhatYoureDoinCartoon', 'YouDontKnowWhatYoureDoinCartoon.mp4'),
    ("Bosko's Soda Fountain", 1931, 'BoskosSodaFountain1931HughHarmanRudolfIsing', "Bosko's Soda Fountain 1931 Hugh Harman, Rudolf Ising.avi"),
    ("Bosko's Fox Hunt", 1931, 'bosko-s-fox-hunt-1931-1080p', 'Bosko s Fox Hunt (1931)_1080p.mp4'),
    ('Red-Headed Baby', 1931, 'RedHeadedBaby1931HughHarmanRudolfIsing', 'Red Headed Baby 1931 Hugh Harman, Rudolf Ising.avi'),
    ('Bosko at the Zoo', 1931, 'BoskoAtTheZoo1932HughHarmanRudolfIsing', 'Bosko at the Zoo 1932 Hugh Harman, Rudolf Ising.avi'),
    ('Pagan Moon', 1932, 'PaganMoon1931HughHarmanRudolfIsing', 'Pagan Moon 1931 Hugh Harman, Rudolf Ising.avi'),
    ('Battling Bosko', 1932, 'BattlingBosko1931HughHarmanRudolfIsing', 'Battling Bosko 1931 Hugh Harman, Rudolf Ising .avi'),
    ('Freddy the Freshman', 1932, 'freddy-the-freshman-1932', 'Freddy the Freshman (1932).mp4'),
    ('Big-Hearted Bosko', 1932, 'BigHeartedBosko1932HughHarmanRudolfIsing', 'Big-Hearted Bosko 1932 Hugh Harman, Rudolf Ising.avi'),
    ('Crosby, Columbo, and Vallee', 1932, 'CrosbyColumboAndVallee1932HughHarmanRudolfIsing', 'Crosby, Columbo and Vallee 1932 Hugh Harman, Rudolf Ising.avi'),
    ("Bosko's Party", 1932, 'BoskosParty1932HughHarmanRudolfIsing', "Bosko's Party  1932 Hugh Harman, Rudolf Ising.avi"),
    ('Goopy Geer', 1932, 'goopy-geer-1932-restored', 'S01E004_Goopy Geer.mp4'),
    ('Bosko and Bruno', 1932, 'BoskoAndBruno1932HughHarmanRudolfIsing', 'Bosko and Bruno 1932 Hugh Harman, Rudolf Ising.avi'),
    ("It's Got Me Again!", 1932, 'ItsGotMeAgain1932HughHarmanRudolfIsing', "It's Got Me Again 1932 Hugh Harman, Rudolf Ising .avi"),
    ('Moonlight for Two', 1932, 'moonlight-for-two-1932', 'Moonlight for Two (1932).mp4'),
    ("Bosko's Dog Race", 1932, 'BoskosDogRace1932HughHarmanRudolfIsing', "Bosko's Dog Race 1932 Hugh Harman, Rudolf Ising.avi"),
    ('The Queen Was in the Parlor', 1932, 'the-queen-was-in-the-parlor-1932', 'The Queen Was in the Parlor (1932).mp4'),
    ('Bosko at the Beach', 1932, 'BoskoAtTheBeach1932HughHarmanRudolfIsing', 'Bosko at the Beach 1932 Hugh Harman, Rudolf Ising.avi'),
    ('I Love a Parade', 1932, 'ILoveAParade1932HughHarmanRudolfIsing', 'I Love a Parade 1932 Hugh Harman, Rudolf Ising.avi'),
    ("Bosko's Store", 1932, 'BoskosStore1932HughHarmanRudolfIsing', "Bosko's Store 1932 Hugh Harman, Rudolf Ising.avi"),
    ('Bosko the Lumberjack', 1932, 'BoskoTheLumberjack1932HughHarmanRudolfIsing', 'Bosko the Lumberjack 1932 Hugh Harman, Rudolf Ising.avi'),
    ("You're Too Careless with Your Kisses!", 1932, 'youre-too-careless-with-your-kisses-1932', "You're Too Careless with Your Kisses! (1932).mp4"),
    ('I Wish I Had Wings', 1932, 'i-wish-i-had-wings-1932', 'I Wish I Had Wings (1932).mp4'),
    ('A Great Big Bunch of You', 1932, 'a-great-big-bunch-of-you-1932_202305', 'A Great Big Bunch of You (1932).mp4'),
    ("Three's a Crowd", 1932, 'threes-a-crowd-1932', "Three's a Crowd (1932).mp4"),
    ('The Shanty Where Santy Claus Lives', 1933, 'the-shanty-where-santy-claus-lives-1933_202305', 'The Shanty Where Santy Claus Lives (1933).mp4'),
    ('Hollywood Capers', 1935, 'hollywood-capers-1935', 'Hollywood Capers (1935).mp4'),
    ('Boom Boom', 1936, 'boom-boom-1936', 'Boom Boom (1936).mp4'),
    ('Westward Whoa', 1936, 'westward-whoa-1936_202305', 'Westward Whoa (1936).mp4'),
    ("Porky's Railroad", 1937, 'porkys-railroad-1937', "Porky's Railroad (1937).mp4"),
    ('Get Rich Quick Porky', 1937, 'get-rich-quick-porky-1937', 'Get Rich Quick Porky (1937).mp4'),
    ("Porky's Garden", 1937, 'porkys-garden-1937', "Porky's Garden (1937).mp4"),
    ('I Wanna Be a Sailor', 1937, 'i-wanna-be-a-sailor_202301', 'I Wanna Be A Sailor (1937) - HD.mp4'),
    ('Have You Got Any Castles?', 1938, 'haveyougotanycastles1938merriemelodieshdcc', 'Have You Got Any Castles - 1938 - Merrie Melodies - (HD + CC).mp4'),
    ('Hamateur Night', 1939, 'looney-tunes-s-1939-e-04-hamateur-night', 'Looney Tunes - S1939E04 - Hamateur Night.mp4'),
    ('Robin Hood Makes Good', 1939, 'robin-hood-makes-good-1939-restored', 'S05E012_Robin Hood Makes Good.mp4'),
    ('Gold Rush Daze', 1939, 'looney-tunes-gold-rush-daze-1939-hd-logoless', 'Looney Tunes - Gold Rush Daze (1939) HD [LOGOLESS].mp4'),
    ('A Day at the Zoo', 1939, 'ADayAtTheZoo1939', 'A Day at the Zoo (1939).mp4'),
    ('Prest-O Change-O', 1939, 'PrestOChangeO1939', 'Prest-O Change-O (1939).MP4'),
    ('Bars and Stripes Forever', 1939, 'looney-tunes-s-1939-e-12-bars-and-stripes-forever', 'Looney Tunes - S1939E12 - Bars and Stripes Forever.mp4'),
    ('Daffy Duck and the Dinosaur', 1939, 'daffy-duck-and-the-dinosaur-1939', 'Daffy Duck and the Dinosaur (1939).mp4'),
    ('The Early Worm Gets the Bird', 1940, 'the-early-worm-gets-the-bird', 'The Early Worm Gets the Bird.mp4'),
    ('Ali-Baba Bound', 1940, 'ali-baba-bound-1940', 'Ali-Baba Bound (1940).mp4'),
    ('The Timid Toreador', 1940, 'TheTimidToreador_73', 'TheTimidToreadorkpf.mp4'),
    ('The Haunted Mouse', 1941, 'the-haunted-mouse-1941', 'The Haunted Mouse (1941).mp4'),
    ('Joe Glow, the Firefly', 1941, 'JoeGlowTheFirefly1941', 'Joe Glow the Firefly (1941).mp4'),
    ("Porky's Bear Facts", 1941, 'porkys-bear-facts-1941', "Porky's Bear Facts (1941).mp4"),
    ("Porky's Preview", 1941, 'porkys-preview-1941_animation', "Porky's Preview (1941).mp4"),
    ("Porky's Ant", 1941, 'PorkysAnt_596', 'PorkysAnt.mp4'),
    ('Farm Frolics', 1941, 'FarmFrolics1941', 'Farm Frolics (1941).MP4'),
    ('A Coy Decoy', 1941, 'a-coy-decoy-1941_202403', 'A Coy Decoy (1941).mp4'),
    ('Meet John Doughboy', 1941, 'meetjohndoughboy1941', 'Meet John Doughboy (1941).mp4'),
    ('We, the Animals - Squeak!', 1941, 'wetheanimalssqueak1941', 'We, the Animals Squeak! (1941).mp4'),
    ('Sport Chumpions', 1941, 'sportchumpions1941', 'Sport Chumpions (1941).mp4'),
    ('The Henpecked Duck', 1941, 'the-henpecked-duck-1941', 'The Henpecked Duck (1941).mp4'),
    ('Notes to You', 1941, 'NotesToYou', 'NotesToYou.mp4'),
    ('Robinson Crusoe Jr.', 1941, 'robinson-crusoe-jr.-1941', 'Robinson Crusoe Jr. (1941).mp4'),
    ('Rookie Revue', 1941, 'RookieRevue1941WW2Cartoon', 'Rookie Revue (1941) WW2 Cartoon.mp4'),
    ("Porky's Midnight Matinee", 1941, 'porkys-midnight-matinee-1941', "Porky's Midnight Matinee (1941).mp4"),
    ("Porky's Pooch", 1941, 'porkyspooch1941', "Porky's Pooch (1941).mp4"),
    ("Porky's Pastry Pirates", 1942, 'porkys-pastry-pirates-1942-restored', 'S08E016_Porkys Pastry Pirates.mp4'),
    ("Who's Who in the Zoo", 1942, 'whos-who-in-the-zoo-1942', "Who's Who in the Zoo (1942).mp4"),
    ("Porky's Cafe", 1942, 'porkys-cafe-1942-restored', 'S08E015_Porkys Cafe.mp4'),
    ('The Wabbit Who Came to Supper', 1942, 'the-wabbit-who-came-to-supper-1942_202605', 'The Wabbit Who Came to Supper (1942).mkv'),
    ('Saps in Chaps', 1942, 'sapsinchaps', '0361- Saps in Chaps (1942).mkv'),
    ('The Wacky Wabbit', 1942, 'the-wacky-wabbit-1942_202605', 'The Wacky Wabbit (1942).mkv'),
    ('Nutty News', 1942, 'nutty-news-1942', 'Nutty News (1942).mp4'),
    ('Hobby Horse-Laffs', 1942, 'hobby-horse-laffs-1942', 'Hobby Horse-Laffs (1942).mp4'),
    ('Gopher Goofy', 1942, 'gopher-goofy-1942', 'Gopher Goofy (1942).mp4'),
    ('Wacky Blackout', 1942, 'WackyBlackout1942WW2Cartoon', 'Wacky Blackout (1942) WW2 Cartoon.mp4'),
    ('Foney Fables', 1942, 'FoneyFables1942', 'Foney Fables (1942).mp4'),
    ("Eatin' on the Cuff", 1942, 'eatin-on-the-cuff-1942-restored', 'S08E001_Eatin on the Cuff.mp4'),
    ('Fresh Hare', 1942, 'fresh-hare-1942_202605', 'Fresh Hare (1942).mkv'),
    ('The Impatient Patient', 1942, 'the-impatient-patient-1942-restored', 'S08E014_The Impatient Patient.mp4'),
    ('Fox Pop', 1942, 'fox-pop_202607', '1942.09.05 - Fox Pop.mp4'),
    ('The Dover Boys', 1942, 'the-dover-boys-at-pimento-university_202402', 'The_Dover_Boys_at_Pimento_University_1080p.webm'),
    ('The Daffy Duckaroo', 1942, 'the-daffy-duckaroo-1942-restored', 'The Daffy Duckaroo (1942).mp4'),
    ('A Tale of Two Kitties', 1942, 'a-tale-of-two-kitties-1942-restored', 'S08E011_A Tale of Two Kitties.mp4'),
    ('Ding Dog Daddy', 1942, 'dingdogdaddy1942looneytunesclassiccartoon', 'Ding Dog Daddy (1942) - Looney Tunes Classic Cartoon.mp4'),
    ('Case of the Missing Hare', 1942, 'case-of-the-missing-hare-1942_202605', 'Case of the Missing Hare (1942).mkv'),
    ('Confusions of a Nutzy Spy', 1943, 'confusions-of-a-nutzy-spy-restored', 'Confusions of a Nutzy Spy (1943).mp4'),
    ('Pigs in a Polka', 1943, 'pigs-in-a-polka-1943-restored', 'S09E006_Pigs in a Polka.mp4'),
    ('To Duck .... or Not to Duck', 1943, 'to-duck-or-not-to-duck-1943_202605', 'To Duck or Not to Duck (1943).mkv'),
    ('The Fifth-Column Mouse', 1943, 'TheFifthColumnMouse1943WW2Cartoon', 'The Fifth Column Mouse (1943) WW2 Cartoon.mp4'),
    ('Hop and Go', 1943, 'hop-and-go-1943_202304', 'Hop and Go (1943).mp4'),
    ('Yankee Doodle Daffy', 1943, 'YankeeDoodleDaffy19431', 'Yankee Doodle Daffy (1943)-1.mp4'),
    ('Wackiki Wabbit', 1943, 'wackiki-wabbit-1943_202605', 'Wackiki Wabbit (1943).mkv'),
    ("Porky Pig's Feat", 1943, 'porky-pigs-feat-1943-restored', 'S09E009_Porky Pigs Feat.mp4'),
    ('Scrap Happy Daffy', 1943, 'scraphappydaffy', 'scraphappydaffy.mp4'),  # the 720p .mov is private
    ('A Corny Concerto', 1943, '1943-9-25-a-corny-concerto', '[1943-9-25] A Corny Concerto @.mkv'),
    ('Falling Hare', 1943, 'falling-hare-1943_202605', 'Falling Hare (1943).mkv'),
    ('Daffy - The Commando', 1943, 'daffy-the-commando-1943-restored', 'Looney Tunes - S1943E26 - Daffy - The Commando.mp4'),
    ("Puss n' Booty", 1943, 'puss-n-booty-1943-restored', 'S09E007_Puss N Booty.mp4'),
    # Private Snafu: made for the US Army, so never under copyright
    ('Coming!! Snafu', 1943, 'private-snafu-no-buddy-atoll', 'Private Snafu - Coming Snafu [i2].mp4'),
    ('Gripes', 1943, 'Pvt.SNAFU.Gripes', 'Pvt.SNAFU.Gripes.avi'),
    ('Spies', 1943, 'private-snafu-spies-1943_202005', 'Private Snafu Spies 1943.mp4'),
    ('The Goldbrick', 1943, 'private-snafu-no-buddy-atoll', 'Private Snafu - The Goldbrick [Pixar].mp4'),
    ('The Infantry Blues', 1943, 'Pvt.SNAFU.TheInfantryBlues', 'Pvt.SNAFU.TheInfantryBlues.avi'),
    ('Fighting Tools', 1943, 'Pvt.SNAFU.FightingTools', 'Pvt.SNAFU.FightingTools.avi'),
    ('The Home Front', 1943, 'private-snafu-no-buddy-atoll', 'Private Snafu - The Home Front [Pixar].mp4'),
    ('Rumors', 1943, 'private-snafu-no-buddy-atoll', 'Private Snafu - Rumors [Pixar].mp4'),
    ('Booby Traps', 1944, 'PrivateSnafuBoobyTraps1944', 'snafu_boobytraps.mp4'),
    ('Snafuperman', 1944, 'PrivateSnafuSnafuperman', 'Private_Snafu_as_Snafuperman.mpg'),
    ('Gas', 1944, 'Pvt.SNAFU.Gas', 'Pvt.SNAFU.Gas.avi'),
    ('The Chow Hound', 1944, 'private-snafu-no-buddy-atoll', 'Private Snafu - The Chow Hound.mp4'),
    ('Censored', 1944, '111-M-1076', '111-M-1076.mp4'),
    ('Outpost', 1944, 'private-snafu-no-buddy-atoll', 'Private Snafu - Outpost.mp4'),
    ('Three Brothers', 1944, 'private-snafu-no-buddy-atoll', 'Private Snafu - The Three Brothers [Pixar].mp4'),
    ('Payday', 1944, 'private-snafu-no-buddy-atoll', 'Private Snafu - Pay Day.mp4'),
    ('Target Snafu', 1944, 'private-snafu-no-buddy-atoll', 'Private Snafu - Target Snafu.mp4'),
    ('In the Aleutians – Isles of Enchantment', 1945, 'private-snafu-no-buddy-atoll', 'Private Snafu - In The Aleutians.mp4'),
    ("It's Murder She Says", 1945, 'Pvt.SNAFU.ItsMurderSheSays', 'Pvt.SNAFU.ItsMurderSheSays.avi'),
    ('Hot Spot', 1945, 'private-snafu-no-buddy-atoll', 'Private Snafu - Hot Spot.mp4'),
    ('Operation Snafu', 1945, 'private-snafu-no-buddy-atoll', 'Private Snafu - Operation Snafu.mp4'),
    ('No Buddy Atoll', 1945, 'Pvt.SNAFU.NoBuddyAtoll', 'Pvt.SNAFU.NoBuddyAtoll.avi'),
    # Mr. Hook: made for the US Navy
    ('The Return of Mr. Hook', 1945, 'mr-hook', 'The Return of Mr. Hook (1945).mp4'),
    ('The Good Egg', 1945, 'mr-hook', 'The Good Egg (1945).mp4'),
    ('Tokyo Woes', 1945, 'TokyoWoes', 'Mr.Hook-03TokyoWoes1945.avi'),
    # Made for the US government
    ('Point Rationing of Foods', 1943, '77354-point-rationing-of-foods', '77354 Point Rationing Of Foods.mov'),
    ('So Much for So Little', 1949, 'so-much-for-so-little-1949', 'So Much for So Little (1949).mp4'),
    ('A Hitch in Time', 1955, 'a-hitch-in-time-1955', 'A Hitch in Time (1955).mp4'),
    ('90 Day Wondering', 1956, '90DayWandering', '90DayWandering.m4v'),
    ("Drafty, Isn't It?", 1957, 'looney-tunes-drafty-isnt-it-1957-hd-logoless', "Looney Tunes - Drafty, Isn't It (1957) HD [LOGOLESS].mp4"),
]

# --- helpers --------------------------------------------------------------------


def request(url, **headers):
    return urllib.request.Request(url, headers={"User-Agent": UA, **headers})


def ia_files(item):
    """The Internet Archive's file list for an item."""
    with urllib.request.urlopen(request(f"https://archive.org/metadata/{item}"), timeout=60) as r:
        return json.load(r).get("files", [])


def ia_url(item, name):
    return f"https://archive.org/download/{item}/{urllib.request.quote(name)}"


def best_mp4(files):
    """The largest h.264 mp4 an item has (its best copy that browsers play)."""
    mp4 = [f for f in files if f["name"].lower().endswith(".mp4")]
    return max(mp4, key=lambda f: int(f.get("size", 0)), default=None)


def remote_size(url):
    try:
        with urllib.request.urlopen(request(url, Range="bytes=0-0"), timeout=60) as r:
            rng = r.headers.get("Content-Range", "")
            return int(rng.rsplit("/", 1)[1]) if "/" in rng else int(r.headers.get("Content-Length", 0))
    except (urllib.error.URLError, ValueError, OSError):
        return 0


def download(url, dest, size_hint=0, tries=5):
    """Download url to dest, resuming a .part file. Skips a finished dest."""
    if os.path.exists(dest) and os.path.getsize(dest) > 0:
        print(f"  have  {os.path.basename(dest)}")
        return True
    os.makedirs(os.path.dirname(dest), exist_ok=True)
    part = dest + ".part"
    for attempt in range(1, tries + 1):
        have = os.path.getsize(part) if os.path.exists(part) else 0
        try:
            req = request(url, **({"Range": f"bytes={have}-"} if have else {}))
            with urllib.request.urlopen(req, timeout=120) as r:
                if have and r.status != 206:  # server ignored the range: start over
                    have = 0
                total = have + int(r.headers.get("Content-Length", 0)) or size_hint
                with open(part, "ab" if have else "wb") as out:
                    done, last = have, 0.0
                    while chunk := r.read(1 << 20):
                        out.write(chunk)
                        done += len(chunk)
                        if time.time() - last > 1:
                            last = time.time()
                            pct = f"{done * 100 // total:3d}%" if total else ""
                            print(f"\r  get   {os.path.basename(dest)}  {done / 1e6:,.0f} MB {pct}   ", end="", flush=True)
            os.replace(part, dest)
            print(f"\r  done  {os.path.basename(dest)}  {os.path.getsize(dest) / 1e6:,.0f} MB        ")
            return True
        except urllib.error.HTTPError as e:
            if e.code == 416:  # range past the end: the .part is complete
                os.replace(part, dest)
                return True
            print(f"\n  HTTP {e.code} for {url} (try {attempt}/{tries})")
            if e.code in (403, 404):
                return False
        except (urllib.error.URLError, OSError, TimeoutError) as e:
            print(f"\n  {e} (try {attempt}/{tries})")
        time.sleep(min(60, 5 * attempt))
    return False


def unzip_one(zpath, dest):
    """Extract the single video in a zip to dest, then delete the zip."""
    with zipfile.ZipFile(zpath) as z:
        member = max(z.infolist(), key=lambda i: i.file_size)
        with z.open(member) as src, open(dest + ".part", "wb") as out:
            shutil.copyfileobj(src, out, 1 << 20)
    os.replace(dest + ".part", dest)
    os.remove(zpath)


def ext_of(name):
    return os.path.splitext(name.removesuffix(".zip"))[1].lower()


def safe(name):
    return re.sub(r'[<>:"/\\|?*]', "", name).strip()


# --- sections -------------------------------------------------------------------


def plan_movies(root):
    for title, src in MOVIES:
        folder = os.path.join(root, "Movies", title)
        if src[0] == "ia":
            url = ia_url(src[1], src[2])
            yield url, os.path.join(folder, title + ext_of(src[2])), None
        else:
            url = src[1]
            dest = os.path.join(folder, title + ext_of(url))
            yield url, dest, (dest + ".zip" if src[0] == "zip" else None)
    for title, lang, url in SUBTITLES:
        yield url, os.path.join(root, "Movies", title, f"{title}.{lang}.srt"), None


RE_EP = re.compile(r"s(\d+)e(\d+)[-_ ]*(.*)\.mp4$", re.I)


def plan_tv(root):
    for folder, show, item in TV:
        for f in ia_files(item):
            m = RE_EP.search(f["name"])
            if not m or f["name"].lower().endswith("_512kb.mp4"):
                continue
            season, ep, rest = int(m[1]), int(m[2]), m[3].replace("_", " ").replace("-", " ").strip()
            name = f"{show} - S{season:02d}E{ep:02d}" + (f" - {safe(rest)}" if rest else "") + ".mp4"
            yield ia_url(item, f["name"]), os.path.join(root, "TV", folder, f"Season {season:02d}", name), None


def plan_commercials(root):
    for title, item in COMMERCIALS:
        f = best_mp4(ia_files(item))
        if f:
            yield ia_url(item, f["name"]), os.path.join(root, "Commercials", title + ".mp4"), None
        else:
            print(f"  skip  {title}: no mp4 in {item}")


def plan_cartoons(root):
    for title, year, item, name in CARTOONS:
        folder = safe(f"{title} ({year})")
        yield ia_url(item, name), os.path.join(root, "Cartoons", folder, folder + ext_of(name)), None


SECTIONS = {"movies": plan_movies, "tv": plan_tv, "cartoons": plan_cartoons, "commercials": plan_commercials}
# Fetched when --only isn't given. The cartoons are tens of GB: ask for them.
DEFAULT = ["movies", "tv", "commercials"]
# Sections that are libraries, and the folder each fills.
LIBRARIES = {"movies": "Movies", "tv": "TV", "cartoons": "Cartoons"}


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("root", help="folder to fill (created if missing)")
    ap.add_argument("--only", nargs="+", choices=SECTIONS, default=DEFAULT, help="sections to fetch (default: %(default)s)")
    ap.add_argument("--list", action="store_true", help="show what would be fetched, with sizes")
    args = ap.parse_args()

    root = os.path.abspath(args.root)
    failed, total = [], 0
    for section in args.only:
        print(f"\n== {section}")
        for url, dest, zpath in SECTIONS[section](root):
            if args.list:
                size = remote_size(url)
                total += size
                print(f"  {size / 1e6:8,.0f} MB  {os.path.relpath(dest, root)}")
                continue
            if os.path.exists(dest):
                print(f"  have  {os.path.basename(dest)}")
                continue
            if zpath:
                if download(url, zpath):
                    print(f"  unzip {os.path.basename(zpath)}")
                    unzip_one(zpath, dest)
                else:
                    failed.append(url)
            elif not download(url, dest):
                failed.append(url)
    if args.list:
        print(f"\nTotal about {total / 1e9:,.1f} GB")
    if failed:
        print("\nFailed (re-run to retry):\n  " + "\n  ".join(failed))
        sys.exit(1)
    if not args.list:
        libs = [os.path.join(root, LIBRARIES[s]) for s in args.only if s in LIBRARIES]
        print("\nDone.")
        if libs:
            note = " (Cartoons as a movie library)" if "cartoons" in args.only else ""
            print(f"Add {', '.join(libs)} as libraries in Couchside{note}.")
        if "commercials" in args.only:
            print(f"{os.path.join(root, 'Commercials')} is filler for virtual channels, not a library.")


if __name__ == "__main__":
    main()
