# -*- coding: utf-8 -*-

import sys
sys.path.append("resolvers")

import flickr
import gyazo
import hatena_fotolife
import imgly
import instagram
import lockerz
import mobypicture
import movapic
import niconico_seiga
import pckles
import photozou
import pixiv
import tinami
import twipple
import twitgoo
import twitpic
import twitvideo
import viame
import yfrog

#アルファベット順

services = [
	flickr.flickr(),
	gyazo.gyazo(),
	hatena_fotolife.hatenaFotolife(),
	instagram.instagram(),
	imgly.imgly(),
	lockerz.lockerz(),
	mobypicture.mobypicture(),
	movapic.movapic(),
	niconico_seiga.niconicoSeiga(),
	pckles.pckles(),
	photozou.photozou(),
	pixiv.pixiv(),
	tinami.tinami(),
	twipple.twipple(),
	twitgoo.twitgoo(),
	twitpic.twitpic(),
	twitvideo.twitvideo(),
	viame.viame(),
	yfrog.yfrog()
]
