export interface CsvTable {
	headers: string[];
	rows: string[][];
	/** Total data rows in the file, including rows beyond the preview cap. */
	totalRows: number;
	/** Rows dropped because of the preview cap. */
	truncated: boolean;
}

const CANDIDATES = [',', ';', '\t'] as const;

/**
 * Splits delimited text into a table. It follows RFC 4180 quoting (doubled
 * quotes inside a quoted field, embedded newlines) and sniffs the delimiter
 * from the first line so semicolon exports from spreadsheets still work.
 */
export function parseCsv(text: string, limit = 500): CsvTable {
	const source = text.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
	if (source.trim() === '') {
		return { headers: [], rows: [], totalRows: 0, truncated: false };
	}

	const delimiter = sniffDelimiter(source);
	const records = splitRecords(source, delimiter);
	const headers = records[0] ?? [];
	const dataRows = records.slice(1).filter((row) => row.some((cell) => cell !== ''));
	const visible = dataRows.slice(0, limit);

	return {
		headers,
		rows: visible,
		totalRows: dataRows.length,
		truncated: dataRows.length > visible.length
	};
}

/** Counts candidates outside quotes on the first few lines. */
function sniffDelimiter(source: string): string {
	const sample = source.split('\n').slice(0, 5).join('\n');
	let best: string = CANDIDATES[0];
	let bestCount = -1;
	for (const candidate of CANDIDATES) {
		const count = countOutsideQuotes(sample, candidate);
		if (count > bestCount) {
			best = candidate;
			bestCount = count;
		}
	}
	return best;
}

function countOutsideQuotes(text: string, delimiter: string): number {
	let quoted = false;
	let count = 0;
	for (let index = 0; index < text.length; index += 1) {
		const char = text[index];
		if (char === '"') {
			if (quoted && text[index + 1] === '"') {
				index += 1;
			} else {
				quoted = !quoted;
			}
			continue;
		}
		if (!quoted && char === delimiter) count += 1;
	}
	return count;
}

function splitRecords(source: string, delimiter: string): string[][] {
	const records: string[][] = [];
	let record: string[] = [];
	let field = '';
	let quoted = false;

	for (let index = 0; index < source.length; index += 1) {
		const char = source[index];

		if (quoted) {
			if (char === '"') {
				if (source[index + 1] === '"') {
					field += '"';
					index += 1;
				} else {
					quoted = false;
				}
			} else {
				field += char;
			}
			continue;
		}

		if (char === '"') {
			quoted = true;
			continue;
		}
		if (char === delimiter) {
			record.push(field);
			field = '';
			continue;
		}
		if (char === '\n') {
			record.push(field);
			records.push(record);
			record = [];
			field = '';
			continue;
		}
		field += char;
	}

	if (field !== '' || record.length > 0) {
		record.push(field);
		records.push(record);
	}
	return records;
}
