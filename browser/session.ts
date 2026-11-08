export const SESSION = '.auth/session.json'

// Unset, sign-in opens a visible browser and waits for someone to type the credentials into the realm's form;
// set (from a secret manager, never a file), the form is filled in unattended
export const credentials =
  process.env.FINTRACE_TEST_USERNAME && process.env.FINTRACE_TEST_PASSWORD
    ? { username: process.env.FINTRACE_TEST_USERNAME, password: process.env.FINTRACE_TEST_PASSWORD }
    : undefined
