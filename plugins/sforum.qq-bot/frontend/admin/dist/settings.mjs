// src/dom.ts
function e(e, t = {}, ...n) {
	let r = document.createElement(e);
	for (let [e, n] of Object.entries(t)) n != null && n !== !1 && (n === !0 ? r.setAttribute(e, "") : r.setAttribute(e, String(n)));
	for (let e of n) e != null && e !== !1 && r.append(e instanceof Node ? e : document.createTextNode(String(e)));
	return r;
}
// node_modules/uqr/dist/index.mjs
var t = /* @__PURE__ */ ((e) => (e[e.Border = -1] = "Border", e[e.Data = 0] = "Data", e[e.Function = 1] = "Function", e[e.Position = 2] = "Position", e[e.Timing = 3] = "Timing", e[e.Alignment = 4] = "Alignment", e))(t || {}), n = [0, 1], r = [1, 0], i = [2, 3], a = [3, 2], o = {
	L: n,
	M: r,
	Q: i,
	H: a
}, s = /^\d*$/, c = /^[A-Z0-9 $%*+./:-]*$/, l = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:", u = 1, d = 40, f = 3, p = 3, m = 40, h = 10, g = [
	[
		-1,
		7,
		10,
		15,
		20,
		26,
		18,
		20,
		24,
		30,
		18,
		20,
		24,
		26,
		30,
		22,
		24,
		28,
		30,
		28,
		28,
		28,
		28,
		30,
		30,
		26,
		28,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30
	],
	[
		-1,
		10,
		16,
		26,
		18,
		24,
		16,
		18,
		22,
		22,
		26,
		30,
		22,
		22,
		24,
		24,
		28,
		28,
		26,
		26,
		26,
		26,
		28,
		28,
		28,
		28,
		28,
		28,
		28,
		28,
		28,
		28,
		28,
		28,
		28,
		28,
		28,
		28,
		28,
		28,
		28
	],
	[
		-1,
		13,
		22,
		18,
		26,
		18,
		24,
		18,
		22,
		20,
		24,
		28,
		26,
		24,
		20,
		30,
		24,
		28,
		28,
		26,
		30,
		28,
		30,
		30,
		30,
		30,
		28,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30
	],
	[
		-1,
		17,
		28,
		22,
		16,
		22,
		28,
		26,
		26,
		24,
		28,
		24,
		28,
		22,
		24,
		24,
		30,
		28,
		28,
		26,
		28,
		30,
		24,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30,
		30
	]
], _ = [
	[
		-1,
		1,
		1,
		1,
		1,
		1,
		2,
		2,
		2,
		2,
		4,
		4,
		4,
		4,
		4,
		6,
		6,
		6,
		6,
		7,
		8,
		8,
		9,
		9,
		10,
		12,
		12,
		12,
		13,
		14,
		15,
		16,
		17,
		18,
		19,
		19,
		20,
		21,
		22,
		24,
		25
	],
	[
		-1,
		1,
		1,
		1,
		2,
		2,
		4,
		4,
		4,
		5,
		5,
		5,
		8,
		9,
		9,
		10,
		10,
		11,
		13,
		14,
		16,
		17,
		17,
		18,
		20,
		21,
		23,
		25,
		26,
		28,
		29,
		31,
		33,
		35,
		37,
		38,
		40,
		43,
		45,
		47,
		49
	],
	[
		-1,
		1,
		1,
		2,
		2,
		4,
		4,
		6,
		6,
		8,
		8,
		8,
		10,
		12,
		16,
		12,
		17,
		16,
		18,
		21,
		20,
		23,
		23,
		25,
		27,
		29,
		34,
		34,
		35,
		38,
		40,
		43,
		45,
		48,
		51,
		53,
		56,
		59,
		62,
		65,
		68
	],
	[
		-1,
		1,
		1,
		2,
		4,
		4,
		4,
		5,
		6,
		8,
		8,
		11,
		11,
		16,
		16,
		18,
		16,
		19,
		21,
		25,
		25,
		25,
		34,
		30,
		32,
		35,
		37,
		40,
		42,
		45,
		48,
		51,
		54,
		57,
		60,
		63,
		66,
		70,
		74,
		77,
		81
	]
], v = class {
	constructor(e, t, n, r) {
		if (this.version = e, this.ecc = t, e < u || e > d) throw RangeError("Version value out of range");
		if (r < -1 || r > 7) throw RangeError("Mask value out of range");
		this.size = e * 4 + 17;
		let i = Array.from({ length: this.size }).fill(!1);
		for (let e = 0; e < this.size; e++) this.modules.push(i.slice()), this.types.push(i.map(() => 0));
		this.drawFunctionPatterns();
		let a = this.addEccAndInterleave(n);
		if (this.drawCodewords(a), r === -1) {
			let e = 1e9;
			for (let t = 0; t < 8; t++) {
				this.applyMask(t), this.drawFormatBits(t);
				let n = this.getPenaltyScore();
				n < e && (r = t, e = n), this.applyMask(t);
			}
		}
		this.mask = r, this.applyMask(r), this.drawFormatBits(r);
	}
	size;
	mask;
	modules = [];
	types = [];
	getModule(e, t) {
		return e >= 0 && e < this.size && t >= 0 && t < this.size && this.modules[t][e];
	}
	drawFunctionPatterns() {
		for (let e = 0; e < this.size; e++) this.setFunctionModule(6, e, e % 2 == 0, t.Timing), this.setFunctionModule(e, 6, e % 2 == 0, t.Timing);
		this.drawFinderPattern(3, 3), this.drawFinderPattern(this.size - 4, 3), this.drawFinderPattern(3, this.size - 4);
		let e = this.getAlignmentPatternPositions(), n = e.length;
		for (let t = 0; t < n; t++) for (let r = 0; r < n; r++) t === 0 && r === 0 || t === 0 && r === n - 1 || t === n - 1 && r === 0 || this.drawAlignmentPattern(e[t], e[r]);
		this.drawFormatBits(0), this.drawVersion();
	}
	drawFormatBits(e) {
		let t = this.ecc[1] << 3 | e, n = t;
		for (let e = 0; e < 10; e++) n = n << 1 ^ (n >>> 9) * 1335;
		let r = (t << 10 | n) ^ 21522;
		for (let e = 0; e <= 5; e++) this.setFunctionModule(8, e, b(r, e));
		this.setFunctionModule(8, 7, b(r, 6)), this.setFunctionModule(8, 8, b(r, 7)), this.setFunctionModule(7, 8, b(r, 8));
		for (let e = 9; e < 15; e++) this.setFunctionModule(14 - e, 8, b(r, e));
		for (let e = 0; e < 8; e++) this.setFunctionModule(this.size - 1 - e, 8, b(r, e));
		for (let e = 8; e < 15; e++) this.setFunctionModule(8, this.size - 15 + e, b(r, e));
		this.setFunctionModule(8, this.size - 8, !0);
	}
	drawVersion() {
		if (this.version < 7) return;
		let e = this.version;
		for (let t = 0; t < 12; t++) e = e << 1 ^ (e >>> 11) * 7973;
		let t = this.version << 12 | e;
		for (let e = 0; e < 18; e++) {
			let n = b(t, e), r = this.size - 11 + e % 3, i = Math.floor(e / 3);
			this.setFunctionModule(r, i, n), this.setFunctionModule(i, r, n);
		}
	}
	drawFinderPattern(e, n) {
		for (let r = -4; r <= 4; r++) for (let i = -4; i <= 4; i++) {
			let a = Math.max(Math.abs(i), Math.abs(r)), o = e + i, s = n + r;
			o >= 0 && o < this.size && s >= 0 && s < this.size && this.setFunctionModule(o, s, a !== 2 && a !== 4, t.Position);
		}
	}
	drawAlignmentPattern(e, n) {
		for (let r = -2; r <= 2; r++) for (let i = -2; i <= 2; i++) this.setFunctionModule(e + i, n + r, Math.max(Math.abs(i), Math.abs(r)) !== 1, t.Alignment);
	}
	setFunctionModule(e, n, r, i = t.Function) {
		this.modules[n][e] = r, this.types[n][e] = i;
	}
	addEccAndInterleave(e) {
		let t = this.version, n = this.ecc;
		if (e.length !== j(t, n)) throw RangeError("Invalid argument");
		let r = _[n[0]][t], i = g[n[0]][t], a = Math.floor(A(t) / 8), o = r - a % r, s = Math.floor(a / r), c = [], l = ie(i);
		for (let t = 0, n = 0; t < r; t++) {
			let r = e.slice(n, n + s - i + (t < o ? 0 : 1));
			n += r.length;
			let a = ae(r, l);
			t < o && r.push(0), c.push(r.concat(a));
		}
		let u = [];
		for (let e = 0; e < c[0].length; e++) c.forEach((t, n) => {
			(e !== s - i || n >= o) && u.push(t[e]);
		});
		return u;
	}
	drawCodewords(e) {
		if (e.length !== Math.floor(A(this.version) / 8)) throw RangeError("Invalid argument");
		let t = 0;
		for (let n = this.size - 1; n >= 1; n -= 2) {
			n === 6 && (n = 5);
			for (let r = 0; r < this.size; r++) for (let i = 0; i < 2; i++) {
				let a = n - i, o = n + 1 & 2 ? r : this.size - 1 - r;
				!this.types[o][a] && t < e.length * 8 && (this.modules[o][a] = b(e[t >>> 3], 7 - (t & 7)), t++);
			}
		}
	}
	applyMask(e) {
		if (e < 0 || e > 7) throw RangeError("Mask value out of range");
		for (let t = 0; t < this.size; t++) for (let n = 0; n < this.size; n++) {
			let r;
			switch (e) {
				case 0:
					r = (n + t) % 2 == 0;
					break;
				case 1:
					r = t % 2 == 0;
					break;
				case 2:
					r = n % 3 == 0;
					break;
				case 3:
					r = (n + t) % 3 == 0;
					break;
				case 4:
					r = (Math.floor(n / 3) + Math.floor(t / 2)) % 2 == 0;
					break;
				case 5:
					r = n * t % 2 + n * t % 3 == 0;
					break;
				case 6:
					r = (n * t % 2 + n * t % 3) % 2 == 0;
					break;
				case 7:
					r = ((n + t) % 2 + n * t % 3) % 2 == 0;
					break;
				default: throw Error("Unreachable");
			}
			!this.types[t][n] && r && (this.modules[t][n] = !this.modules[t][n]);
		}
	}
	getPenaltyScore() {
		let e = 0;
		for (let t = 0; t < this.size; t++) {
			let n = !1, r = 0, i = [
				0,
				0,
				0,
				0,
				0,
				0,
				0
			];
			for (let a = 0; a < this.size; a++) this.modules[t][a] === n ? (r++, r === 5 ? e += f : r > 5 && e++) : (this.finderPenaltyAddHistory(r, i), n || (e += this.finderPenaltyCountPatterns(i) * m), n = this.modules[t][a], r = 1);
			e += this.finderPenaltyTerminateAndCount(n, r, i) * m;
		}
		for (let t = 0; t < this.size; t++) {
			let n = !1, r = 0, i = [
				0,
				0,
				0,
				0,
				0,
				0,
				0
			];
			for (let a = 0; a < this.size; a++) this.modules[a][t] === n ? (r++, r === 5 ? e += f : r > 5 && e++) : (this.finderPenaltyAddHistory(r, i), n || (e += this.finderPenaltyCountPatterns(i) * m), n = this.modules[a][t], r = 1);
			e += this.finderPenaltyTerminateAndCount(n, r, i) * m;
		}
		for (let t = 0; t < this.size - 1; t++) for (let n = 0; n < this.size - 1; n++) {
			let r = this.modules[t][n];
			r === this.modules[t][n + 1] && r === this.modules[t + 1][n] && r === this.modules[t + 1][n + 1] && (e += p);
		}
		let t = 0;
		for (let e of this.modules) t = e.reduce((e, t) => e + +!!t, t);
		let n = this.size * this.size, r = Math.ceil(Math.abs(t * 20 - n * 10) / n) - 1;
		return e += r * h, e;
	}
	getAlignmentPatternPositions() {
		if (this.version === 1) return [];
		{
			let e = Math.floor(this.version / 7) + 2, t = this.version === 32 ? 26 : Math.ceil((this.version * 4 + 4) / (e * 2 - 2)) * 2, n = [6];
			for (let r = this.size - 7; n.length < e; r -= t) n.splice(1, 0, r);
			return n;
		}
	}
	finderPenaltyCountPatterns(e) {
		let t = e[1], n = t > 0 && e[2] === t && e[3] === t * 3 && e[4] === t && e[5] === t;
		return (n && e[0] >= t * 4 && e[6] >= t ? 1 : 0) + (n && e[6] >= t * 4 && e[0] >= t ? 1 : 0);
	}
	finderPenaltyTerminateAndCount(e, t, n) {
		return e && (this.finderPenaltyAddHistory(t, n), t = 0), t += this.size, this.finderPenaltyAddHistory(t, n), this.finderPenaltyCountPatterns(n);
	}
	finderPenaltyAddHistory(e, t) {
		t[0] === 0 && (e += this.size), t.pop(), t.unshift(e);
	}
};
function y(e, t, n) {
	if (t < 0 || t > 31 || e >>> t) throw RangeError("Value out of range");
	for (let r = t - 1; r >= 0; r--) n.push(e >>> r & 1);
}
function b(e, t) {
	return !!(e >>> t & 1);
}
var x = class {
	constructor(e, t, n) {
		if (this.mode = e, this.numChars = t, this.bitData = n, t < 0) throw RangeError("Invalid argument");
		this.bitData = n.slice();
	}
	getData() {
		return this.bitData.slice();
	}
}, S = [
	1,
	10,
	12,
	14
], C = [
	2,
	9,
	11,
	13
], w = [
	4,
	8,
	16,
	16
];
function T(e, t) {
	return e[Math.floor((t + 7) / 17) + 1];
}
function E(e) {
	let t = [];
	for (let n of e) y(n, 8, t);
	return new x(w, e.length, t);
}
function ee(e) {
	if (!D(e)) throw RangeError("String contains non-numeric characters");
	let t = [];
	for (let n = 0; n < e.length;) {
		let r = Math.min(e.length - n, 3);
		y(Number.parseInt(e.substring(n, n + r), 10), r * 3 + 1, t), n += r;
	}
	return new x(S, e.length, t);
}
function te(e) {
	if (!O(e)) throw RangeError("String contains unencodable characters in alphanumeric mode");
	let t = [], n = 0;
	for (; n + 2 <= e.length; n += 2) {
		let r = l.indexOf(e.charAt(n)) * 45;
		r += l.indexOf(e.charAt(n + 1)), y(r, 11, t);
	}
	return n < e.length && y(l.indexOf(e.charAt(n)), 6, t), new x(C, e.length, t);
}
function ne(e) {
	return e === "" ? [] : D(e) ? [ee(e)] : O(e) ? [te(e)] : [E(k(e))];
}
function D(e) {
	return s.test(e);
}
function O(e) {
	return c.test(e);
}
function re(e, t) {
	let n = 0;
	for (let r of e) {
		let e = T(r.mode, t);
		if (r.numChars >= 1 << e) return Infinity;
		n += 4 + e + r.bitData.length;
	}
	return n;
}
function k(e) {
	e = encodeURI(e);
	let t = [];
	for (let n = 0; n < e.length; n++) e.charAt(n) === "%" ? (t.push(Number.parseInt(e.substring(n + 1, n + 3), 16)), n += 2) : t.push(e.charCodeAt(n));
	return t;
}
function A(e) {
	if (e < u || e > d) throw RangeError("Version number out of range");
	let t = (16 * e + 128) * e + 64;
	if (e >= 2) {
		let n = Math.floor(e / 7) + 2;
		t -= (25 * n - 10) * n - 55, e >= 7 && (t -= 36);
	}
	return t;
}
function j(e, t) {
	return Math.floor(A(e) / 8) - g[t[0]][e] * _[t[0]][e];
}
function ie(e) {
	if (e < 1 || e > 255) throw RangeError("Degree out of range");
	let t = [];
	for (let n = 0; n < e - 1; n++) t.push(0);
	t.push(1);
	let n = 1;
	for (let r = 0; r < e; r++) {
		for (let e = 0; e < t.length; e++) t[e] = M(t[e], n), e + 1 < t.length && (t[e] ^= t[e + 1]);
		n = M(n, 2);
	}
	return t;
}
function ae(e, t) {
	let n = t.map((e) => 0);
	for (let r of e) {
		let e = r ^ n.shift();
		n.push(0), t.forEach((t, r) => n[r] ^= M(t, e));
	}
	return n;
}
function M(e, t) {
	if (e >>> 8 || t >>> 8) throw RangeError("Byte out of range");
	let n = 0;
	for (let r = 7; r >= 0; r--) n = n << 1 ^ (n >>> 7) * 285, n ^= (t >>> r & 1) * e;
	return n;
}
function oe(e, t, n = 1, o = 40, s = -1, c = !0) {
	if (!(u <= n && n <= o && o <= d) || s < -1 || s > 7) throw RangeError("Invalid value");
	let l, f;
	for (l = n;; l++) {
		let n = j(l, t) * 8, r = re(e, l);
		if (r <= n) {
			f = r;
			break;
		}
		if (l >= o) throw RangeError("Data too long");
	}
	for (let e of [
		r,
		i,
		a
	]) c && f <= j(l, e) * 8 && (t = e);
	let p = [];
	for (let t of e) {
		y(t.mode[0], 4, p), y(t.numChars, T(t.mode, l), p);
		for (let e of t.getData()) p.push(e);
	}
	let m = j(l, t) * 8;
	y(0, Math.min(4, m - p.length), p), y(0, (8 - p.length % 8) % 8, p);
	for (let e = 236; p.length < m; e ^= 253) y(e, 8, p);
	let h = Array.from({ length: Math.ceil(p.length / 8) }, () => 0);
	return p.forEach((e, t) => h[t >>> 3] |= e << 7 - (t & 7)), new v(l, t, h, s);
}
function se(e, t) {
	let { ecc: n = "L", boostEcc: r = !1, minVersion: i = 1, maxVersion: a = 40, maskPattern: s = -1, border: c = 1 } = t || {}, l = typeof e == "string" ? ne(e) : Array.isArray(e) ? [E(e)] : void 0;
	if (!l) throw Error(`uqr only supports encoding string and binary data, but got: ${typeof e}`);
	let u = oe(l, o[n], i, a, s, r), d = ce({
		version: u.version,
		maskPattern: u.mask,
		size: u.size,
		data: u.modules,
		types: u.types
	}, c);
	return t?.invert && (d.data = d.data.map((e) => e.map((e) => !e))), t?.onEncoded?.(d), d;
}
function ce(e, n = 1) {
	if (!n) return e;
	let { size: r } = e, i = r + n * 2;
	e.size = i, e.data.forEach((e) => {
		for (let t = 0; t < n; t++) e.unshift(!1), e.push(!1);
	});
	for (let t = 0; t < n; t++) e.data.unshift(Array.from({ length: i }, (e) => !1)), e.data.push(Array.from({ length: i }, (e) => !1));
	let a = t.Border;
	e.types.forEach((e) => {
		for (let t = 0; t < n; t++) e.unshift(a), e.push(a);
	});
	for (let t = 0; t < n; t++) e.types.unshift(Array.from({ length: i }, (e) => a)), e.types.push(Array.from({ length: i }, (e) => a));
	return e;
}
function N(e) {
	return e.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}
function le(e, t = {}) {
	let n = se(e, t), { pixelSize: r = 10, whiteColor: i = "white", blackColor: a = "black" } = t, o = n.size * r, s = n.size * r, c = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${s} ${o}">`, l = [];
	for (let e = 0; e < n.size; e++) for (let t = 0; t < n.size; t++) {
		let i = t * r, a = e * r;
		n.data[e][t] && l.push(`M${i},${a}h${r}v${r}h-${r}z`);
	}
	return c += `<rect fill="${N(i)}" width="${s}" height="${o}"/>`, c += `<path fill="${N(a)}" d="${l.join("")}"/>`, c += "</svg>", c;
}
// src/bind-card.ts
var ue = 1500, de = 500, fe = 300, pe = 3;
function me(e) {
	try {
		let t = le(e, {
			ecc: "M",
			border: 2,
			pixelSize: 4,
			blackColor: "#111111",
			whiteColor: "#ffffff"
		}), n = new DOMParser().parseFromString(t, "image/svg+xml");
		return n.documentElement ? document.importNode(n.documentElement, !0) : null;
	} catch {
		return null;
	}
}
function he(e) {
	let t = Math.max(0, Math.floor(e)), n = Math.floor(t / 60);
	return `${String(n).padStart(2, "0")}:${String(t % 60).padStart(2, "0")}`;
}
function P(t) {
	let n = "qqbot-bind-title", r = e("div", { class: "qqbot-bind__body" }), i = e("p", {
		class: "qqbot-live",
		role: "status",
		"aria-live": "polite"
	}), a = e("section", {
		class: "qqbot-card qqbot-bind",
		"aria-labelledby": n
	}, e("header", { class: "qqbot-card__header" }, e("h3", {
		class: "qqbot-card__title",
		id: n
	}, t.t("bind.title")), e("p", { class: "qqbot-card__hint" }, t.t("bind.hint"))), r, i), o = t.getValue("app_id") !== "" || t.secretSaved ? "ready" : "idle", s = t.getValue("app_id"), c = "", l = "", u = 0, d = 0, f = 0, p, m, h = null, g = null, _ = () => {
		p !== void 0 && window.clearInterval(p), m !== void 0 && window.clearInterval(m), p = void 0, m = void 0, h = null;
	}, v = () => {
		h && (h.textContent = t.t("bind.expires", { time: he((u - Date.now()) / 1e3) }));
	}, y = (t, n, r) => {
		let i = e("button", {
			type: "button",
			class: `qqbot-btn qqbot-btn--${n}`
		}, t);
		return i.addEventListener("click", r), i;
	}, b = () => y(t.t("bind.manual"), "ghost", () => t.focus("app_id")), x = () => {
		_(), a.removeAttribute("aria-busy");
		let n = [], c = null;
		if (o === "idle") n.push(e("div", { class: "qqbot-bind__actions" }, y(t.t("bind.start"), "primary", () => {
			E();
		}), b()));
		else if (o === "ready") s ? n.push(e("p", { class: "qqbot-bind__current" }, t.t("bind.boundId", { id: s }))) : t.secretSaved && n.push(e("p", { class: "qqbot-bind__current" }, t.t("bind.secretReady"))), n.push(e("div", { class: "qqbot-bind__actions" }, y(t.t("bind.rebind"), "secondary", () => {
			E();
		}), b()));
		else if (o === "starting") a.setAttribute("aria-busy", "true"), n.push(e("p", { class: "qqbot-muted" }, t.t("bind.starting")));
		else if (o === "waiting") {
			h = e("p", { class: "qqbot-bind__countdown" }), v();
			let r = e("div", {
				class: "qqbot-qr",
				role: "img",
				"aria-label": t.t("bind.title")
			});
			g && r.append(g), n.push(e("div", { class: "qqbot-bind__layout" }, r, e("div", { class: "qqbot-bind__side" }, e("p", { class: "qqbot-bind__waiting" }, t.t("bind.waiting")), h, e("a", {
				class: "qqbot-bind__link",
				href: l,
				target: "_blank",
				rel: "noopener noreferrer"
			}, t.t("bind.openLink")), e("div", { class: "qqbot-bind__actions" }, y(t.t("bind.retry"), "secondary", () => {
				E();
			}), b()))));
		} else if (o === "saving") a.setAttribute("aria-busy", "true"), n.push(e("p", { class: "qqbot-muted" }, t.t("bind.saving")));
		else if (o === "bound") n.push(e("p", { class: "qqbot-bind__done" }, t.t("bind.bound")), e("p", { class: "qqbot-muted" }, t.t("bind.boundId", { id: s })), e("div", { class: "qqbot-bind__actions" }, y(t.t("bind.rebind"), "secondary", () => {
			E();
		}), b())), i.textContent = t.t("bind.bound");
		else if (o === "expired") {
			let r = y(t.t("bind.retry"), "primary", () => {
				E();
			});
			c = r, n.push(e("p", { class: "qqbot-bind__error" }, t.t("bind.expired")), e("div", { class: "qqbot-bind__actions" }, r, b())), i.textContent = t.t("bind.expired");
		} else {
			let r = y(t.t("bind.retry"), "secondary", () => {
				E();
			});
			c = r, n.push(e("p", { class: "qqbot-bind__error" }, t.t("bind.error")), e("div", { class: "qqbot-bind__actions" }, r, b())), i.textContent = t.t("bind.error");
		}
		r.replaceChildren(...n), c && c.focus();
	}, S = (e) => {
		let t = typeof e == "number" ? e : Number(e);
		return Number.isFinite(t) && t > 0 ? t : fe;
	}, C = () => {
		_(), o = "expired", x();
	}, w = async (e, n) => {
		_(), o = "saving", x();
		let r = typeof n.appId == "string" ? n.appId.trim() : "", a = typeof n.appSecret == "string" ? n.appSecret : "";
		if (!r || !a) {
			e === d && !t.disposed && (o = "error", x());
			return;
		}
		if (t.setValue("app_id", r), t.setValue("app_secret", a, !1), await t.bridge.settings.save(), e !== d || t.disposed) return;
		let c = t.bridge.settings.values();
		c.app_id !== r || (c.app_secret ?? "") !== "" ? (o = "error", x(), i.textContent = t.t("error.saveFailed")) : (s = r, t.markSecretSaved(), t.syncFromBridge(), o = "bound", x(), t.toast("success", t.t("bind.bound"), t.t("bind.boundId", { id: r })));
	}, T = async (e) => {
		if (e !== d || t.disposed || o !== "waiting") return;
		if (Date.now() >= u) {
			C();
			return;
		}
		let n;
		try {
			n = await t.request("/bind/poll", {
				method: "POST",
				body: { taskId: c }
			});
		} catch {
			if (e !== d || t.disposed) return;
			f += 1, f >= pe && (o = "error", x());
			return;
		}
		if (e !== d || t.disposed) return;
		f = 0;
		let r = typeof n?.appId == "string" && n.appId.trim() !== "", i = typeof n?.appSecret == "string" && n.appSecret !== "";
		r && i ? await w(e, n) : n?.status === "expired" && C();
	}, E = async () => {
		_();
		let e = ++d;
		f = 0, o = "starting", x();
		try {
			let n = await t.request("/bind/start", {
				method: "POST",
				body: {}
			});
			if (e !== d || t.disposed) return;
			let r = typeof n?.taskId == "string" ? n.taskId : "", i = typeof n?.connectUrl == "string" ? n.connectUrl : "";
			if (!r || !i) {
				o = "error", x();
				return;
			}
			c = r, l = i, u = Date.now() + S(n.expiresInSeconds) * 1e3, g = me(l), o = "waiting", x(), m = window.setInterval(() => {
				o === "waiting" && (Date.now() >= u ? C() : v());
			}, de), p = window.setInterval(() => {
				T(e);
			}, ue);
		} catch {
			if (e !== d || t.disposed) return;
			o = "error", x();
		}
	};
	return x(), {
		root: a,
		destroy: () => {
			d += 1, _();
		}
	};
}
// src/footer.ts
var F = 4e3;
function I(e) {
	return (/* @__PURE__ */ new Date()).toLocaleTimeString(e.zh ? "zh-CN" : "en-US", {
		hour: "2-digit",
		minute: "2-digit"
	});
}
function L(t) {
	let n = "qqbot-footer-title", r = e("pre", { class: "qqbot-output" });
	r.hidden = !0;
	let i = e("p", {
		class: "qqbot-live",
		role: "status",
		"aria-live": "polite"
	}), a = (t, n) => {
		let r = e("button", {
			type: "button",
			class: `qqbot-btn qqbot-btn--${t}`
		});
		return r.addEventListener("click", n), r;
	}, o = null, s = !1, c, l = a("primary", () => {
		y();
	}), u = a("ghost", () => v()), d = a("secondary", () => {
		x();
	}), f = a("secondary", () => {
		S();
	}), p = e("section", {
		class: "qqbot-card qqbot-footer",
		"aria-labelledby": n
	}, e("header", { class: "qqbot-card__header" }, e("h3", {
		class: "qqbot-card__title",
		id: n
	}, t.t("footer.title")), e("p", { class: "qqbot-card__hint" }, t.t("footer.hint"))), e("div", { class: "qqbot-footer__row" }, e("div", { class: "qqbot-footer__group" }, l, u), e("div", { class: "qqbot-footer__group" }, d, f)), r, i), m = () => {
		l.disabled = o !== null, u.disabled = o !== null, d.disabled = o !== null, f.disabled = o !== null, l.textContent = o === "save" ? t.t("footer.saveBusy") : t.t("footer.save"), u.textContent = s ? t.t("footer.resetArmed") : t.t("footer.reset"), d.textContent = o === "test" ? t.t("footer.testBusy") : t.t("footer.test"), f.textContent = o === "status" ? t.t("footer.statusBusy") : t.t("footer.status");
	}, h = (e) => {
		let n = t.getValue("app_secret");
		return n.length < 4 ? e : e.split(n).join("***");
	}, g = (e, t) => {
		let n = e.trim();
		r.hidden = n === "", r.classList.toggle("qqbot-output--bad", t === "bad"), r.textContent = n;
	}, _ = () => {
		c !== void 0 && window.clearTimeout(c), c = void 0, s = !1, m();
	};
	function v() {
		o || (s ? b() : (s = !0, m(), c = window.setTimeout(_, F)));
	}
	async function y() {
		if (!o) {
			o = "save", m();
			try {
				await t.bridge.settings.save(), t.syncFromBridge(), i.textContent = `${t.t("footer.savedAt", { time: I(t) })}`;
			} catch {
				g(t.t("error.generic"), "bad"), i.textContent = t.t("error.generic");
			} finally {
				o = null, m();
			}
		}
	}
	async function b() {
		if (!o) {
			_(), o = "reset", m();
			try {
				await t.bridge.settings.reset(), t.syncFromBridge(), i.textContent = t.t("footer.resetDone", { time: I(t) });
			} catch {
				g(t.t("error.generic"), "bad"), i.textContent = t.t("error.generic");
			} finally {
				o = null, m();
			}
		}
	}
	async function x() {
		if (!o) {
			o = "test", m();
			try {
				let e = await t.request("/test-push", {
					method: "POST",
					body: {}
				}), n = e?.ok === !0, r = h(typeof e?.message == "string" ? e.message : t.t("error.generic"));
				g(r, n ? "ok" : "bad"), i.textContent = n ? t.t("footer.testOk") : t.t("footer.testFail"), t.toast(n ? "success" : "error", n ? t.t("footer.testOk") : t.t("footer.testFail"), r);
			} catch {
				g(t.t("error.generic"), "bad"), i.textContent = t.t("footer.testFail"), t.toast("error", t.t("footer.testFail"), t.t("error.generic"));
			} finally {
				o = null, m();
			}
		}
	}
	async function S() {
		if (!o) {
			o = "status", m();
			try {
				let e = await t.request("/status", {
					method: "POST",
					body: {}
				}), n = h(typeof e?.text == "string" ? e.text : t.t("error.generic"));
				g(n, "ok"), i.textContent = t.t("footer.statusOk"), t.toast("success", t.t("footer.statusOk"));
			} catch {
				g(t.t("error.generic"), "bad"), i.textContent = t.t("footer.statusFail"), t.toast("error", t.t("footer.statusFail"), t.t("error.generic"));
			} finally {
				o = null, m();
			}
		}
	}
	return m(), {
		root: p,
		destroy: () => {
			c !== void 0 && window.clearTimeout(c), c = void 0;
		}
	};
}
// src/form.ts
var R = {
	group_ids: 4,
	category_filter: 4,
	message_template: 10
}, ge = {
	excerpt_limit: {
		min: 0,
		max: 5e3,
		step: 10
	},
	max_message_chars: {
		min: 0,
		max: 5e3,
		step: 50
	},
	daily_push_cap_per_group: {
		min: 0,
		max: 1e3,
		step: 10
	},
	max_pushes_per_minute: {
		min: 1,
		max: 600,
		step: 1
	},
	max_retries: {
		min: 0,
		max: 10,
		step: 1
	},
	send_timeout_seconds: {
		min: 1,
		max: 120,
		step: 1
	}
};
function z(e) {
	return e instanceof HTMLInputElement && e.type === "checkbox" ? e.checked ? "true" : "false" : e.value;
}
function B(t, n) {
	return t.description ? e("p", {
		class: "qqbot-field__desc",
		id: n
	}, t.description) : null;
}
function V(t, n) {
	let r = t.item(n);
	if (!r) return null;
	let i = `sforum-qqbot-${n}`, a = `${i}-desc`, o = r.description ? { "aria-describedby": a } : {};
	if (r.type === "boolean") {
		let s = e("input", {
			type: "checkbox",
			id: i,
			...o
		});
		s.checked = t.getValue(n) === "true";
		let c = e("div", {
			class: "qqbot-field qqbot-field--boolean",
			"data-key": n
		}, e("label", {
			class: "qqbot-check",
			for: i
		}, s, e("span", {}, r.label || n)), B(r, a));
		return t.registerControl(n, s, () => z(s)), {
			root: c,
			control: s,
			item: r
		};
	}
	let s, c = null;
	if (r.type === "select") {
		let a = r.options ?? [], l = e("select", {
			id: i,
			class: "qqbot-input",
			...o
		});
		for (let t of a) l.append(e("option", { value: t.value }, t.label || t.value));
		l.value = a.some((e) => e.value === t.getValue(n)) ? t.getValue(n) : a[0]?.value ?? "", s = l;
		let u = e("p", { class: "qqbot-field__hint" }), d = () => {
			let e = a.find((e) => e.value === l.value);
			u.textContent = e?.description || "", u.hidden = !e?.description;
		};
		d(), t.onInput([n], d), c = u;
	} else if (r.type === "textarea") {
		let a = e("textarea", {
			id: i,
			rows: R[n] ?? 3,
			class: n === "message_template" ? "qqbot-input qqbot-input--mono" : "qqbot-input",
			placeholder: r.placeholder || "",
			spellcheck: "false",
			...o
		});
		a.value = t.getValue(n), s = a;
	} else if (r.type === "number") {
		let r = ge[n], a = e("input", {
			type: "number",
			id: i,
			class: "qqbot-input",
			inputmode: "numeric",
			...r ?? {},
			...o
		});
		a.value = t.getValue(n), s = a;
	} else if (r.type === "secret") {
		let n = e("input", {
			type: "password",
			id: i,
			class: "qqbot-input",
			autocomplete: "new-password",
			spellcheck: "false",
			placeholder: t.secretSaved ? t.t("field.keepSecret") : t.t("field.secretPlaceholder"),
			...o
		});
		n.value = "", s = n;
	} else {
		let a = e("input", {
			type: "text",
			id: i,
			class: "qqbot-input",
			autocomplete: "off",
			spellcheck: "false",
			placeholder: r.placeholder || "",
			...o
		});
		a.value = t.getValue(n), s = a;
	}
	let l = r.type === "secret" ? e("span", { class: "qqbot-badge" }, t.t("field.saved")) : null;
	l && (l.hidden = !t.secretSaved);
	let u = e("div", {
		class: r.width === "full" ? "qqbot-field qqbot-field--full" : "qqbot-field",
		"data-key": n
	}, e("div", { class: "qqbot-field__head" }, e("label", {
		class: "qqbot-field__label",
		for: i
	}, r.label || n), l), s, c, B(r, a));
	if (r.type === "secret") {
		let e = s;
		t.onSecretSavedChange(() => {
			e.placeholder = t.secretSaved ? t.t("field.keepSecret") : t.t("field.secretPlaceholder"), l && (l.hidden = !t.secretSaved);
		});
	}
	return t.registerControl(n, s, () => z(s)), {
		root: u,
		control: s,
		item: r
	};
}
function H(t, n) {
	let r = `qqbot-section-${n.id}`, i = e("div", { class: "qqbot-grid" });
	for (let e of n.keys) {
		let n = V(t, e);
		n && i.append(n.root);
	}
	return e("section", {
		class: "qqbot-card",
		"aria-labelledby": r
	}, e("header", { class: "qqbot-card__header" }, e("h3", {
		class: "qqbot-card__title",
		id: r
	}, n.title), n.hint ? e("p", { class: "qqbot-card__hint" }, n.hint) : null), i);
}
// src/groups-card.ts
var U = "group_ids", _e = 6, ve = {
	year: "numeric",
	month: "2-digit",
	day: "2-digit",
	hour: "2-digit",
	minute: "2-digit"
};
function W(e) {
	return typeof e == "string" ? e : "";
}
function G(e) {
	return typeof e == "number" && Number.isFinite(e) && e > 0 ? Math.floor(e) : 0;
}
function K(e) {
	return e.split(",").map((e) => e.trim());
}
function q(e, t) {
	return e.split("\n").some((e) => K(e).some((e) => e === t));
}
function ye(e, t) {
	return `${e}${e === "" || e.endsWith("\n") ? "" : "\n"}${t}`;
}
function be(e, t) {
	let n = [];
	for (let r of e.split("\n")) {
		let e = K(r);
		if (!e.some((e) => e === t)) {
			n.push(r);
			continue;
		}
		let i = e.filter((e) => e !== t && e !== "");
		i.length > 0 && n.push(i.join(", "));
	}
	let r = n.join("\n");
	return e.endsWith("\n") && r !== "" && !r.endsWith("\n") ? `${r}\n` : r;
}
function xe(e) {
	if (!Array.isArray(e)) return [];
	let t = [];
	for (let n of e) {
		if (!n || typeof n != "object") continue;
		let e = n, r = W(e.openid).trim();
		if (r === "") continue;
		let i = W(e.name).trim(), a = W(e.lastSeenAt), o = Date.parse(a);
		t.push({
			openid: r,
			name: i,
			nameUnavailable: e.nameUnavailable === !0,
			count: G(e.count),
			lastSeen: Number.isFinite(o) ? o : -Infinity,
			lastSeenText: a
		});
	}
	return t.sort((e, t) => t.lastSeen - e.lastSeen);
}
function Se(t) {
	let n = "qqbot-groups-title", r = e("p", {
		class: "qqbot-groups__status",
		role: "status",
		"aria-live": "polite"
	}), i = e("p", { class: "qqbot-groups__note" });
	i.hidden = !0;
	let a = e("ul", { class: "qqbot-groups__list" });
	a.hidden = !0;
	let o = e("button", {
		type: "button",
		class: "qqbot-btn qqbot-btn--ghost qqbot-groups__refresh"
	}, t.t("groups.refresh")), s = e("div", {
		class: "qqbot-groups",
		role: "group",
		"aria-labelledby": n
	}, e("header", { class: "qqbot-groups__head" }, e("div", { class: "qqbot-groups__heading" }, e("h4", {
		class: "qqbot-groups__title",
		id: n
	}, t.t("groups.title")), e("p", { class: "qqbot-groups__hint" }, t.t("groups.hint"))), o), i, a, r), c = /* @__PURE__ */ new Map(), l = !1, u = 0, d = (e) => {
		i.textContent = e, i.hidden = e === "";
	}, f = (e) => {
		r.textContent = e;
	}, p = () => {
		let e = t.getValue(U);
		for (let [t, n] of c) n.checked = q(e, t);
	}, m = (e, n) => {
		let r = t.getValue(U);
		t.setValue(U, n.checked ? ye(r, e) : be(r, e)), p();
	}, h = (e) => e.name !== "" && !e.nameUnavailable ? e.name : `${e.openid.slice(0, _e)}…`, g = (e) => {
		let n = Date.parse(e.lastSeenText);
		return Number.isFinite(n) ? new Date(n).toLocaleString(t.zh ? "zh-CN" : "en-US", ve) : e.lastSeenText;
	}, _ = (n) => {
		c.clear(), a.replaceChildren(), n.forEach((n, r) => {
			let i = `qqbot-group-${r}`, o = e("input", {
				type: "checkbox",
				id: i,
				class: "qqbot-groups__box"
			});
			o.checked = q(t.getValue(U), n.openid), o.addEventListener("change", () => m(n.openid, o)), c.set(n.openid, o), a.append(e("li", { class: "qqbot-groups__row" }, o, e("label", {
				class: "qqbot-groups__label",
				for: i
			}, e("span", {
				class: "qqbot-groups__name",
				title: n.openid
			}, h(n)), e("span", { class: "qqbot-groups__meta" }, t.t("groups.rowMeta", {
				time: g(n),
				count: n.count
			})))));
		}), a.hidden = n.length === 0;
	}, v = (e) => {
		let n = xe(e?.groups), r = e?.claimEnabled !== !1, i = G(e?.recordingLimit);
		_(n), r ? n.length === 0 ? d(t.t("groups.empty")) : i > 0 && n.length >= i ? d(t.t("groups.limit", { n: i })) : d("") : d(t.t("groups.claimOff")), f(t.t("groups.updated", { n: n.length }));
	}, y = () => {
		c.clear(), a.replaceChildren(), a.hidden = !0, d(t.t("groups.unavailable")), f(t.t("groups.unavailable"));
	}, b = async () => {
		if (l || t.disposed) return;
		l = !0;
		let e = ++u;
		o.disabled = !0, o.textContent = t.t("groups.refreshing"), f(t.t("groups.loading"));
		try {
			let n = await t.bridge.request("/groups/list", {
				method: "POST",
				body: {},
				responseMode: "body"
			});
			if (e !== u || t.disposed) return;
			v(n);
		} catch {
			if (e !== u || t.disposed) return;
			y();
		} finally {
			e === u && !t.disposed && (l = !1, o.disabled = !1, o.textContent = t.t("groups.refresh"));
		}
	};
	return o.addEventListener("click", () => {
		b();
	}), t.onInput([U], p), b(), {
		root: s,
		destroy: () => {
			u += 1, c.clear(), s.remove();
		}
	};
}
// src/i18n.ts
var Ce = {
	"bind.title": "绑定 QQ 机器人",
	"bind.hint": "用手机 QQ 扫码，AppID 与 AppSecret 会自动填好并保存；也可以手动填写。",
	"bind.start": "扫码绑定",
	"bind.manual": "手动填写凭据",
	"bind.retry": "重新生成二维码",
	"bind.rebind": "重新绑定",
	"bind.secretReady": "凭据已配置（AppSecret 已保存，AppID 可在下方补全）",
	"bind.starting": "正在生成二维码…",
	"bind.waiting": "用手机 QQ 扫码，然后在手机上确认授权。",
	"bind.expires": "{time} 后过期",
	"bind.openLink": "二维码扫不出来？在手机 QQ 中打开这个链接",
	"bind.saving": "扫码成功，正在保存凭据…",
	"bind.bound": "已绑定并保存",
	"bind.boundId": "当前 AppID：{id}",
	"bind.expired": "二维码已过期，请重新生成。",
	"bind.error": "绑定流程没有走通，请重试；也可以直接手动填写凭据。",
	"group.connection": "连接",
	"group.connection.hint": "机器人应用凭据。AppSecret 只写不读，留空表示保留已保存的值。",
	"group.targets": "推送目标",
	"group.targets.hint": "推给哪些群、哪些分类，以及什么时候停。",
	"group.template": "消息模板",
	"group.template.hint": "一条推送长什么样。改完保存，下一次推送就生效。",
	"group.limits": "限速与重试",
	"group.limits.hint": "插件侧的护栏，避免撞上 QQ 的频控与日额度。",
	"group.probe": "排错",
	"group.probe.hint": "排查问题时才需要打开，平时保持关闭。",
	"group.other": "其他设置",
	"group.other.hint": "清单里新增的分组。",
	"groups.title": "已识别的群",
	"groups.hint": "机器人收到的群事件会记在这里。勾选即写入下面的群 ID 列表，取消即删掉那一行；手写的行不受影响。",
	"groups.refresh": "刷新",
	"groups.refreshing": "刷新中…",
	"groups.loading": "正在读取…",
	"groups.empty": "还没有识别到群。把机器人拉进目标群，或在群里 @ 它一次，再点刷新。",
	"groups.claimOff": "群 ID 自动识别没有打开，插件不会记录群事件。勾选下方的「启用群 ID 自动识别」，再点刷新。",
	"groups.unavailable": "群列表暂不可用，仍可手动填写群 ID",
	"groups.limit": "只保留最近 {n} 个",
	"groups.rowMeta": "最近活跃 {time} · {count} 次事件",
	"groups.updated": "群列表已刷新，共 {n} 个",
	"field.saved": "已保存",
	"field.keepSecret": "已保存，留空表示不修改",
	"field.secretPlaceholder": "填写 AppSecret",
	"template.presets": "模板预设（点一下覆盖当前模板内容）",
	"template.presetDefault": "默认",
	"template.presetCompact": "精简",
	"template.presetNotice": "公告风",
	"template.tokens": "可用变量（点击插入到光标处）",
	"template.tokensHint": "中文别名同样有效，例如 {{标题}}。不认识的变量会原样保留，方便你对照检查。",
	"template.preview": "预览",
	"template.previewSavedBadge": "将实际发送",
	"template.previewDraftBadge": "草稿预览",
	"template.previewDirty": "草稿与已保存配置不同，当前预览是按已保存配置渲染的；点保存后才会这样发出去。",
	"template.previewDirtyDraft": "当前预览按草稿渲染，还没有保存；点保存后才会这样发出去。",
	"template.previewUseDraft": "按草稿预览",
	"template.previewUseSaved": "按已保存预览",
	"template.previewEmpty": "（模板为空，渲染结果也是空的）",
	"template.previewFailed": "预览暂时不可用；保存后推送仍会正常渲染。",
	"template.truncated": "已截断",
	"template.unknown": "未知变量",
	"template.count": "{n} 字符",
	"template.live": "预览已更新，{n} 字符",
	"footer.title": "保存与测试",
	"footer.hint": "测试消息与推送状态都按已保存的配置执行，不改设置、不写投递记录。",
	"footer.save": "保存",
	"footer.saveBusy": "正在保存…",
	"footer.reset": "恢复默认",
	"footer.resetArmed": "再点一次确认恢复",
	"footer.test": "发一条测试消息",
	"footer.testBusy": "正在发送…",
	"footer.status": "推送状态",
	"footer.statusBusy": "正在读取…",
	"footer.savedAt": "已提交保存 · {time}",
	"footer.resetDone": "已提交恢复默认 · {time}",
	"footer.testOk": "测试消息已发出",
	"footer.testFail": "测试消息没有发出",
	"footer.statusOk": "状态已刷新",
	"footer.statusFail": "读取状态失败",
	"error.routeTitle": "先在插件列表启用本插件，再回来绑定。",
	"error.routeDetail": "如果插件已经启用，请确认后端已随保存重启；原始错误不在这里展示。",
	"error.permissionTitle": "当前账号没有这个插件的管理权限。",
	"error.permissionDetail": "需要 sforum.qq-bot.manage 权限；请超级管理员分配，或换一个有权限的账号再试。",
	"error.rateTitle": "操作太频繁，被插件侧的限流挡住了。",
	"error.rateDetail": "等几秒再试；连续点「重新生成二维码」或反复触发预览会撞同一道限流。",
	"error.serviceTitle": "插件服务暂时不可用。",
	"error.serviceDetail": "后端可能刚重启或返回了错误；稍后再试，原始错误不会显示在这里。",
	"error.generic": "操作没有成功，请重试。",
	"error.saveFailed": "扫码成功，但自动保存没有生效，请检查后在页面底部手动保存。"
}, we = {
	"bind.title": "Connect the QQ bot",
	"bind.hint": "Scan with mobile QQ and AppID plus AppSecret are filled in and saved for you. You can also type them by hand.",
	"bind.start": "Scan to connect",
	"bind.manual": "Enter credentials by hand",
	"bind.retry": "Generate a new code",
	"bind.rebind": "Connect again",
	"bind.secretReady": "Credentials configured (AppSecret saved; add the AppID below)",
	"bind.starting": "Generating the QR code…",
	"bind.waiting": "Scan the code with mobile QQ, then confirm on the phone.",
	"bind.expires": "expires in {time}",
	"bind.openLink": "Cannot scan it? Open this link in mobile QQ",
	"bind.saving": "Scanned. Saving the credentials…",
	"bind.bound": "Connected and saved",
	"bind.boundId": "Current AppID: {id}",
	"bind.expired": "The code expired. Generate a new one.",
	"bind.error": "The connection flow did not go through. Try again, or enter the credentials by hand.",
	"group.connection": "Connection",
	"group.connection.hint": "Bot application credentials. The AppSecret is write-only; leave it blank to keep the saved value.",
	"group.targets": "Delivery targets",
	"group.targets.hint": "Which groups and categories get pushed, and when to stop.",
	"group.template": "Message template",
	"group.template.hint": "What one push looks like. Save and the next push uses it.",
	"group.limits": "Rate limits and retries",
	"group.limits.hint": "Plugin-side guardrails that keep you under the QQ rate limits and daily quota.",
	"group.probe": "Troubleshooting",
	"group.probe.hint": "Turn it on only while chasing a problem.",
	"group.other": "Other settings",
	"group.other.hint": "A group added to the manifest.",
	"groups.title": "Recognized groups",
	"groups.hint": "Group events the bot received show up here. Ticking writes that group into the group ID list below, unticking removes that line; lines you typed yourself are left alone.",
	"groups.refresh": "Refresh",
	"groups.refreshing": "Refreshing…",
	"groups.loading": "Reading…",
	"groups.empty": "No group recognized yet. Add the bot to the target group, or mention it once in the group, then refresh.",
	"groups.claimOff": "Group ID discovery is off, so no group events are recorded. Turn on \"Enable group ID discovery\" below, then refresh.",
	"groups.unavailable": "The group list is unavailable right now; you can still type group IDs by hand",
	"groups.limit": "Only the latest {n} are kept",
	"groups.rowMeta": "Last seen {time} · {count} events",
	"groups.updated": "Group list refreshed: {n} total",
	"field.saved": "Saved",
	"field.keepSecret": "Saved; leave blank to keep it",
	"field.secretPlaceholder": "Enter the AppSecret",
	"template.presets": "Presets (a click replaces the current template)",
	"template.presetDefault": "Default",
	"template.presetCompact": "Compact",
	"template.presetNotice": "Announcement",
	"template.tokens": "Variables (click to insert at the cursor)",
	"template.tokensHint": "Chinese aliases work too, e.g. {{标题}}. Unknown variables are left untouched so you can spot typos.",
	"template.preview": "Preview",
	"template.previewSavedBadge": "What gets sent",
	"template.previewDraftBadge": "Draft preview",
	"template.previewDirty": "The draft differs from the saved settings, so this preview renders the saved ones; only saving makes it go out like this.",
	"template.previewDirtyDraft": "This preview renders the draft, which is not saved yet; save it to go out like this.",
	"template.previewUseDraft": "Preview the draft",
	"template.previewUseSaved": "Preview the saved settings",
	"template.previewEmpty": "(the template is empty, so the rendered message is empty)",
	"template.previewFailed": "The preview is unavailable right now; pushes still render it.",
	"template.truncated": "Truncated",
	"template.unknown": "Unknown variables",
	"template.count": "{n} characters",
	"template.live": "Preview updated, {n} characters",
	"footer.title": "Save and test",
	"footer.hint": "The test message and the status report both run against the saved settings; neither changes settings nor writes a delivery record.",
	"footer.save": "Save",
	"footer.saveBusy": "Saving…",
	"footer.reset": "Restore defaults",
	"footer.resetArmed": "Click again to confirm",
	"footer.test": "Send a test message",
	"footer.testBusy": "Sending…",
	"footer.status": "Delivery status",
	"footer.statusBusy": "Reading…",
	"footer.savedAt": "Save submitted · {time}",
	"footer.resetDone": "Restore submitted · {time}",
	"footer.testOk": "The test message went out",
	"footer.testFail": "The test message did not go out",
	"footer.statusOk": "Status refreshed",
	"footer.statusFail": "Could not read the status",
	"error.routeTitle": "Enable this plugin in the plugin list, then come back.",
	"error.routeDetail": "If the plugin is already enabled, check that the backend restarted after the last save; raw errors are not shown here.",
	"error.permissionTitle": "This account cannot manage the plugin.",
	"error.permissionDetail": "It needs the sforum.qq-bot.manage permission; ask a super administrator to grant it, or switch to an account that has it.",
	"error.rateTitle": "Too many requests; the plugin rate limit stopped this one.",
	"error.rateDetail": "Wait a few seconds and retry; repeated QR regeneration or previews hit the same limit.",
	"error.serviceTitle": "The plugin service is unavailable right now.",
	"error.serviceDetail": "The backend may have just restarted or returned an error; try again shortly. Raw errors are never shown here.",
	"error.generic": "That did not work. Please try again.",
	"error.saveFailed": "Scanned, but the automatic save did not take effect. Check the form and save it at the bottom of the page."
};
function Te(e) {
	let t = (e || "").toLowerCase().startsWith("zh"), n = t ? Ce : we;
	return {
		zh: t,
		t: (e, t) => {
			let r = n[e];
			if (t) for (let [e, n] of Object.entries(t)) r = r.replaceAll(`{${e}}`, String(n));
			return r;
		}
	};
}
// src/template.ts
var Ee = [
	{
		token: "title",
		alias: "标题"
	},
	{
		token: "author",
		alias: "作者"
	},
	{
		token: "category",
		alias: "分类"
	},
	{
		token: "tags",
		alias: "标签"
	},
	{
		token: "excerpt",
		alias: "摘要"
	},
	{
		token: "url",
		alias: "链接"
	},
	{
		token: "topicId",
		alias: "主题ID"
	},
	{
		token: "date",
		alias: "日期"
	},
	{
		token: "signature",
		alias: "签名"
	}
], J = [
	"message_template",
	"message_format",
	"signature",
	"excerpt_limit",
	"max_message_chars"
], De = 300, Oe = [
	{
		label: "template.presetDefault",
		text: null
	},
	{
		label: "template.presetCompact",
		text: "【新帖】{{title}}\n{{url}}"
	},
	{
		label: "template.presetNotice",
		text: "【社区公告】{{title}}\n\n{{excerpt}}\n\n分类：{{category}}｜作者：{{author}}\n详情：{{url}}\n{{signature}}"
	}
];
function Y(e) {
	if (e.trim() === "") return;
	let t = Number.parseInt(e, 10);
	return Number.isFinite(t) ? t : void 0;
}
function ke(e, t) {
	let n = document.activeElement === e, r = n ? e.selectionStart ?? e.value.length : e.value.length, i = n ? e.selectionEnd ?? r : r;
	e.value = `${e.value.slice(0, r)}${t}${e.value.slice(i)}`, e.dispatchEvent(new Event("input", { bubbles: !0 }));
	let a = r + t.length;
	e.focus(), e.setSelectionRange(a, a);
}
function Ae(t, n) {
	let r = e("div", { class: "qqbot-chips" });
	for (let i of Ee) {
		let a = `{{${i.token}}}`, o = e("button", {
			type: "button",
			class: "qqbot-chip",
			title: t.zh ? `${a} / {{${i.alias}}}` : a,
			"aria-label": t.zh ? `${a}（${i.alias}）` : a
		}, e("code", {}, a), t.zh ? e("span", { class: "qqbot-chip__alias" }, i.alias) : null);
		o.addEventListener("click", () => ke(n(), a)), r.append(o);
	}
	return e("div", { class: "qqbot-tokens" }, e("p", { class: "qqbot-tokens__title" }, t.t("template.tokens")), r, e("p", { class: "qqbot-tokens__hint" }, t.t("template.tokensHint")));
}
function X(t, n) {
	let r = "qqbot-presets-title", i = e("div", {
		class: "qqbot-presets__row",
		role: "group",
		"aria-labelledby": r
	}), a = [], o = () => {
		for (let e of a) e.setAttribute("aria-pressed", "false");
	}, s = t.item("message_template")?.default ?? "";
	for (let r of Oe) {
		let c = r.text ?? s;
		if (c === "") continue;
		let l = e("button", {
			type: "button",
			class: "qqbot-preset",
			"aria-pressed": "false"
		}, t.t(r.label));
		l.addEventListener("click", () => {
			let e = n();
			e.value !== c && (e.value = c, e.dispatchEvent(new Event("input", { bubbles: !0 }))), o(), l.setAttribute("aria-pressed", "true"), e.focus(), e.setSelectionRange(c.length, c.length);
		}), a.push(l), i.append(l);
	}
	return n().addEventListener("input", o), e("div", { class: "qqbot-presets" }, e("p", {
		class: "qqbot-presets__title",
		id: r
	}, t.t("template.presets")), i);
}
function je(t) {
	let n = "qqbot-preview-title", r = e("pre", { class: "qqbot-preview__body" }), i = e("span", { class: "qqbot-count" }), a = e("span", { class: "qqbot-badge" }), o = e("span", { class: "qqbot-badge qqbot-badge--warn" }, t.t("template.truncated"));
	o.hidden = !0;
	let s = e("p", { class: "qqbot-preview__unknown" });
	s.hidden = !0;
	let c = e("p", { class: "qqbot-preview__dirty" }), l = e("button", {
		type: "button",
		class: "qqbot-btn qqbot-btn--ghost qqbot-preview__mode"
	}), u = e("div", { class: "qqbot-preview__notice" }, c, l);
	u.hidden = !0;
	let d = e("p", {
		class: "qqbot-sr-only",
		role: "status",
		"aria-live": "polite"
	}), f = e("div", {
		class: "qqbot-preview",
		"aria-labelledby": n
	}, e("header", { class: "qqbot-preview__head" }, e("h4", {
		class: "qqbot-preview__title",
		id: n
	}, t.t("template.preview")), a, i, o), u, r, s, d), p, m = 0, h = "saved", g = "", _ = !1, v = () => J.some((e) => (t.getValue(e) ?? "") !== (t.savedValues[e] ?? "")), y = () => {
		let e = v(), n = e && h === "draft";
		return a.textContent = t.t(n ? "template.previewDraftBadge" : "template.previewSavedBadge"), a.classList.toggle("qqbot-badge--quiet", n), u.hidden = !e, l.hidden = !e, e && (c.textContent = t.t(n ? "template.previewDirtyDraft" : "template.previewDirty"), l.textContent = t.t(n ? "template.previewUseSaved" : "template.previewUseDraft")), n;
	}, b = (e) => {
		if (!e) return { source: "saved" };
		let n = Y(t.getValue("excerpt_limit")), r = Y(t.getValue("max_message_chars"));
		return {
			source: "draft",
			template: t.getValue("message_template"),
			format: t.getValue("message_format") || "text",
			signature: t.getValue("signature"),
			...n === void 0 ? {} : { excerptLimit: n },
			...r === void 0 ? {} : { maxMessageChars: r }
		};
	}, x = (e, n) => {
		let a = e === "";
		r.classList.toggle("is-empty", a), r.textContent = a ? t.t("template.previewEmpty") : e, i.textContent = t.t("template.count", { n }), d.textContent = t.t("template.live", { n });
	}, S = async () => {
		let n = ++m, a = y(), c = b(a), l = JSON.stringify(c);
		if (!(l === g && _)) {
			g = l, _ = !1;
			try {
				let r = await t.bridge.request("/preview", {
					method: "POST",
					body: c,
					responseMode: "body"
				});
				if (n !== m || t.disposed) return;
				let i = typeof r?.text == "string" ? r.text : "", a = Array.isArray(r?.unknownVariables) ? r.unknownVariables.filter((e) => typeof e == "string" && e !== "") : [], l = typeof r?.characters == "number" && Number.isFinite(r.characters) && r.characters >= 0 ? Math.floor(r.characters) : Array.from(i).length;
				return x(i, l), o.hidden = r?.truncated !== !0, s.hidden = a.length === 0, a.length > 0 && s.replaceChildren(document.createTextNode(`${t.t("template.unknown")}: `), ...a.map((t) => e("code", { class: "qqbot-preview__var" }, `{{${t}}}`))), _ = !0, !0;
			} catch {
				return n !== m || t.disposed || (o.hidden = !0, s.hidden = !0, r.classList.add("is-empty"), r.textContent = t.t("template.previewFailed"), i.textContent = ""), !1;
			}
		}
	}, C = (e = De) => {
		p !== void 0 && window.clearTimeout(p), p = window.setTimeout(() => {
			p = void 0, S();
		}, e);
	};
	return l.addEventListener("click", () => {
		h = h === "draft" ? "saved" : "draft", C(0);
	}), t.onSavedChange(() => {
		h = "saved", g = "", _ = !1, C(0);
	}), t.onInput(J, () => {
		y();
	}), t.onInput(J, C), y(), C(0), {
		root: f,
		stop: () => {
			p !== void 0 && window.clearTimeout(p), p = void 0;
		}
	};
}
function Me(e) {
	let t = H(e, {
		id: "template",
		keys: ["message_format", "excerpt_limit"],
		title: e.t("group.template"),
		hint: e.t("group.template.hint")
	}), n = t.querySelector(".qqbot-grid"), r = V(e, "message_template");
	if (n && r) {
		let t = () => r.control;
		r.root.insertBefore(X(e, t), r.control), r.root.insertBefore(Ae(e, t), r.control), n.append(r.root);
	}
	let i = je(e);
	n && n.append(i.root);
	let a = V(e, "signature");
	return n && a && n.append(a.root), {
		root: t,
		stop: i.stop
	};
}
// src/app.ts
var Z = {
	connection: "group.connection",
	targets: "group.targets",
	template: "group.template",
	limits: "group.limits",
	probe: "group.probe"
}, Q = {
	connection: "group.connection.hint",
	targets: "group.targets.hint",
	template: "group.template.hint",
	limits: "group.limits.hint",
	probe: "group.probe.hint"
}, $ = [
	"connection",
	"targets",
	"template",
	"limits",
	"probe"
], Ne = [
	"message_template",
	"message_format",
	"signature",
	"excerpt_limit",
	"max_message_chars"
], Pe = {
	route: {
		title: "error.routeTitle",
		detail: "error.routeDetail"
	},
	permission: {
		title: "error.permissionTitle",
		detail: "error.permissionDetail"
	},
	rate: {
		title: "error.rateTitle",
		detail: "error.rateDetail"
	},
	service: {
		title: "error.serviceTitle",
		detail: "error.serviceDetail"
	}
};
function Fe(e) {
	if (!e || typeof e != "object") return;
	let t = e;
	for (let e of [
		t.status,
		t.statusCode,
		t.response?.status,
		t.cause?.status
	]) if (typeof e == "number" && e >= 400 && e <= 599) return e;
}
function Ie(e) {
	let t = Fe(e);
	return t === void 0 || t === 404 ? "route" : t === 401 || t === 403 ? "permission" : t === 429 ? "rate" : "service";
}
function Le(t) {
	let n = Te(t.locale), r = n.t, i = t.settings.items ?? [], a = /* @__PURE__ */ new Map(), o = {}, s = {};
	for (let e of i) {
		a.set(e.key, e);
		let t = typeof e.value == "string" ? e.value : e.default ?? "";
		o[e.key] = t, s[e.key] = t;
	}
	let c = e("strong"), l = e("span", { class: "qqbot-banner__detail" }), u = e("div", {
		class: "qqbot-banner",
		role: "alert"
	}, c, l);
	u.hidden = !0;
	let d = (e) => {
		let t = Pe[e];
		c.textContent = r(t.title), l.textContent = r(t.detail), u.hidden = !1;
	}, f = /* @__PURE__ */ new Map(), p = /* @__PURE__ */ new Map(), m = /* @__PURE__ */ new Set(), h = /* @__PURE__ */ new Set(), g = (e) => {
		let t = p.get(e);
		if (t) for (let e of t) e();
	}, _ = (e, t) => {
		e instanceof HTMLInputElement && e.type === "checkbox" ? e.checked = t === "true" : e.value !== t && (e.value = t);
	}, v = {
		bridge: t,
		i18n: n,
		t: r,
		zh: n.zh,
		draft: o,
		savedValues: s,
		disposed: !1,
		secretSaved: i.some((e) => e.key === "app_secret" && e.secretSet === !0),
		item: (e) => a.get(e),
		async request(e, n = {}) {
			try {
				let r = await t.request(e, {
					...n,
					responseMode: "body"
				});
				return u.hidden = !0, r;
			} catch (e) {
				throw d(Ie(e)), e;
			}
		},
		toast: (e, n, r) => t.toast({
			kind: e,
			title: n,
			description: r
		}),
		getValue: (e) => o[e] ?? "",
		setValue: (e, n, r = !0) => {
			o[e] = n, t.settings.updateValue(e, n);
			let i = f.get(e);
			r && i && _(i, n);
		},
		registerControl: (e, n, r) => {
			f.set(e, n);
			let i = () => {
				let n = r();
				o[e] !== n && (o[e] = n, t.settings.updateValue(e, n)), g(e);
			};
			n.addEventListener("input", i), n.addEventListener("change", i);
		},
		onInput: (e, t) => {
			for (let n of e) {
				let e = p.get(n) ?? /* @__PURE__ */ new Set();
				e.add(t), p.set(n, e);
			}
		},
		focus: (e) => f.get(e)?.focus(),
		syncFromBridge: () => {
			let e = t.settings.values(), n = !1;
			for (let t of Object.keys(o)) {
				let r = e[t];
				if (typeof r != "string" || r === o[t]) continue;
				o[t] = r;
				let i = f.get(t);
				i && _(i, r), Ne.includes(t) && (n = !0);
			}
			for (let t of Object.keys(s)) {
				let n = e[t];
				typeof n == "string" && (s[t] = n);
			}
			for (let e of h) e();
			n && g("message_template");
		},
		onSavedChange: (e) => {
			h.add(e);
		},
		markSecretSaved: () => {
			v.secretSaved = !0;
			for (let e of m) e();
		},
		onSecretSavedChange: (e) => {
			m.add(e);
		}
	}, y = P(v), b = Me(v), x = Se(v), S = L(v), C = /* @__PURE__ */ new Map();
	for (let e of i) {
		let t = (e.groupId || "").trim() || "other", n = C.get(t);
		n ? n.push(e) : C.set(t, [e]);
	}
	let w = [...$.filter((e) => C.has(e)), ...[...C.keys()].filter((e) => !$.includes(e))], T = [];
	for (let e of w) {
		let t = C.get(e) ?? [];
		if (e === "template") {
			T.push(b.root);
			continue;
		}
		let n = e === "other" ? r("group.other") : Z[e] ? r(Z[e]) : e, i = e === "other" ? r("group.other.hint") : Q[e] ? r(Q[e]) : void 0, a = H(v, {
			id: e,
			keys: t.map((e) => e.key),
			title: n,
			hint: i
		});
		if (e === "targets") {
			let e = a.querySelector("[data-key=\"group_ids\"]");
			e?.parentElement && e.parentElement.insertBefore(x.root, e);
		}
		T.push(a);
	}
	let E = e("div", { class: "qqbot-settings" }, u, y.root, ...T, S.root);
	return {
		root: E,
		destroy: () => {
			v.disposed = !0, y.destroy(), b.stop(), x.destroy(), S.destroy(), E.remove();
		}
	};
}
// src/settings.ts
var Re = 1;
function ze(e, t) {
	let n = Le(t);
	return e.append(n.root), () => n.destroy();
}
export { Re as apiVersion, ze as mount };
