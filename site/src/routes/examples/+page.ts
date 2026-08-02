import { getAllExamples, getGroupedExamples } from '$lib/content';

export const load = () => ({
	examples: getAllExamples(),
	groups: getGroupedExamples()
});
