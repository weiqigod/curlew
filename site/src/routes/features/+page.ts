import { getAllExamples } from '$lib/content';

export const load = () => ({ examples: getAllExamples() });
