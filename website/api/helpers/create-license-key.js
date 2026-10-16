module.exports = {


  friendlyName: 'Create license key',


  description: '',


  inputs: {

    numberOfHosts: {
      type: 'number',
      required: true,
    },

    organization: {
      type: 'string',
      required: true,
    },

    expiresAt: {
      type: 'number',
      required: true,
      description: 'A JS timestamp representing when this license will expire.'
    },

    partnerName: {
      type: 'string',
      description: 'The name of the partner who will be reselling this genereated license.',
      extendedDescription: 'This input is only used by the admin license generator tool.',
    }

  },


  exits: {

    success: {
      outputType: 'string',
    },

    invalidNumberOfHosts: {
      description: 'The provided numberOfHosts is out of bounds.'
    },

    invalidOrganization: {
      description: 'The provided organization value is invalid.'
    },

    invalidExpiresAt: {
      description: 'The provided expiresAt is out of bounds.'
    },

  },


  fn: async function ({numberOfHosts, organization, expiresAt, partnerName}) {

    let jwt = require('jsonwebtoken');

    if (!Number.isInteger(numberOfHosts) || numberOfHosts <= 0 || numberOfHosts > 1000000) {
      throw 'invalidNumberOfHosts';
    }

    if (typeof organization !== 'string' || organization.trim() === '' || organization.length > 200) {
      throw 'invalidOrganization';
    }

    let nowInMs = Date.now();
    let maxExpiresAtInMs = nowInMs + (10 * 365 * 24 * 60 * 60 * 1000); // ten years from now
    if (!Number.isFinite(expiresAt) || expiresAt <= nowInMs || expiresAt > maxExpiresAtInMs) {
      throw 'invalidExpiresAt';
    }

    let expirationTimestampInSeconds = Math.floor(expiresAt / 1000);
    let token = jwt.sign(
      {
        iss: 'Fleet Device Management Inc.',
        exp: expirationTimestampInSeconds,
        sub: organization,
        devices: numberOfHosts,
        note: 'Created with Fleet License key dispenser',
        tier: 'premium',
        partner: partnerName // If this value is undefined, it will not be included in the generated token.
      },
      {
        key: sails.config.custom.licenseKeyGeneratorPrivateKey,
        passphrase: sails.config.custom.licenseKeyGeneratorPassphrase
      },
      { algorithm: 'ES256' }
    );


    return token;

  }


};


