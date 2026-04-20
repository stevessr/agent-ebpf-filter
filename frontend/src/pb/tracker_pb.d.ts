import * as $protobuf from "protobufjs";
import Long = require("long");
/** Namespace pb. */
export namespace pb {

    /** Properties of a RegisterRequest. */
    interface IRegisterRequest {

        /** RegisterRequest pid */
        pid?: (number|null);
    }

    /** Represents a RegisterRequest. */
    class RegisterRequest implements IRegisterRequest {

        /**
         * Constructs a new RegisterRequest.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IRegisterRequest);

        /** RegisterRequest pid. */
        public pid: number;

        /**
         * Creates a new RegisterRequest instance using the specified properties.
         * @param [properties] Properties to set
         * @returns RegisterRequest instance
         */
        public static create(properties?: pb.IRegisterRequest): pb.RegisterRequest;

        /**
         * Encodes the specified RegisterRequest message. Does not implicitly {@link pb.RegisterRequest.verify|verify} messages.
         * @param message RegisterRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IRegisterRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified RegisterRequest message, length delimited. Does not implicitly {@link pb.RegisterRequest.verify|verify} messages.
         * @param message RegisterRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IRegisterRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a RegisterRequest message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns RegisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.RegisterRequest;

        /**
         * Decodes a RegisterRequest message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns RegisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.RegisterRequest;

        /**
         * Verifies a RegisterRequest message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a RegisterRequest message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns RegisterRequest
         */
        public static fromObject(object: { [k: string]: any }): pb.RegisterRequest;

        /**
         * Creates a plain object from a RegisterRequest message. Also converts values to other types if specified.
         * @param message RegisterRequest
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.RegisterRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this RegisterRequest to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for RegisterRequest
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a RegisterResponse. */
    interface IRegisterResponse {

        /** RegisterResponse success */
        success?: (boolean|null);

        /** RegisterResponse message */
        message?: (string|null);
    }

    /** Represents a RegisterResponse. */
    class RegisterResponse implements IRegisterResponse {

        /**
         * Constructs a new RegisterResponse.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IRegisterResponse);

        /** RegisterResponse success. */
        public success: boolean;

        /** RegisterResponse message. */
        public message: string;

        /**
         * Creates a new RegisterResponse instance using the specified properties.
         * @param [properties] Properties to set
         * @returns RegisterResponse instance
         */
        public static create(properties?: pb.IRegisterResponse): pb.RegisterResponse;

        /**
         * Encodes the specified RegisterResponse message. Does not implicitly {@link pb.RegisterResponse.verify|verify} messages.
         * @param message RegisterResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IRegisterResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified RegisterResponse message, length delimited. Does not implicitly {@link pb.RegisterResponse.verify|verify} messages.
         * @param message RegisterResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IRegisterResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a RegisterResponse message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns RegisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.RegisterResponse;

        /**
         * Decodes a RegisterResponse message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns RegisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.RegisterResponse;

        /**
         * Verifies a RegisterResponse message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a RegisterResponse message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns RegisterResponse
         */
        public static fromObject(object: { [k: string]: any }): pb.RegisterResponse;

        /**
         * Creates a plain object from a RegisterResponse message. Also converts values to other types if specified.
         * @param message RegisterResponse
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.RegisterResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this RegisterResponse to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for RegisterResponse
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an UnregisterRequest. */
    interface IUnregisterRequest {

        /** UnregisterRequest pid */
        pid?: (number|null);
    }

    /** Represents an UnregisterRequest. */
    class UnregisterRequest implements IUnregisterRequest {

        /**
         * Constructs a new UnregisterRequest.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IUnregisterRequest);

        /** UnregisterRequest pid. */
        public pid: number;

        /**
         * Creates a new UnregisterRequest instance using the specified properties.
         * @param [properties] Properties to set
         * @returns UnregisterRequest instance
         */
        public static create(properties?: pb.IUnregisterRequest): pb.UnregisterRequest;

        /**
         * Encodes the specified UnregisterRequest message. Does not implicitly {@link pb.UnregisterRequest.verify|verify} messages.
         * @param message UnregisterRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IUnregisterRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified UnregisterRequest message, length delimited. Does not implicitly {@link pb.UnregisterRequest.verify|verify} messages.
         * @param message UnregisterRequest message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IUnregisterRequest, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an UnregisterRequest message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns UnregisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.UnregisterRequest;

        /**
         * Decodes an UnregisterRequest message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns UnregisterRequest
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.UnregisterRequest;

        /**
         * Verifies an UnregisterRequest message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an UnregisterRequest message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns UnregisterRequest
         */
        public static fromObject(object: { [k: string]: any }): pb.UnregisterRequest;

        /**
         * Creates a plain object from an UnregisterRequest message. Also converts values to other types if specified.
         * @param message UnregisterRequest
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.UnregisterRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this UnregisterRequest to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for UnregisterRequest
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an UnregisterResponse. */
    interface IUnregisterResponse {

        /** UnregisterResponse success */
        success?: (boolean|null);

        /** UnregisterResponse message */
        message?: (string|null);
    }

    /** Represents an UnregisterResponse. */
    class UnregisterResponse implements IUnregisterResponse {

        /**
         * Constructs a new UnregisterResponse.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IUnregisterResponse);

        /** UnregisterResponse success. */
        public success: boolean;

        /** UnregisterResponse message. */
        public message: string;

        /**
         * Creates a new UnregisterResponse instance using the specified properties.
         * @param [properties] Properties to set
         * @returns UnregisterResponse instance
         */
        public static create(properties?: pb.IUnregisterResponse): pb.UnregisterResponse;

        /**
         * Encodes the specified UnregisterResponse message. Does not implicitly {@link pb.UnregisterResponse.verify|verify} messages.
         * @param message UnregisterResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IUnregisterResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified UnregisterResponse message, length delimited. Does not implicitly {@link pb.UnregisterResponse.verify|verify} messages.
         * @param message UnregisterResponse message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IUnregisterResponse, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an UnregisterResponse message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns UnregisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.UnregisterResponse;

        /**
         * Decodes an UnregisterResponse message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns UnregisterResponse
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.UnregisterResponse;

        /**
         * Verifies an UnregisterResponse message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an UnregisterResponse message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns UnregisterResponse
         */
        public static fromObject(object: { [k: string]: any }): pb.UnregisterResponse;

        /**
         * Creates a plain object from an UnregisterResponse message. Also converts values to other types if specified.
         * @param message UnregisterResponse
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.UnregisterResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this UnregisterResponse to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for UnregisterResponse
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an Event. */
    interface IEvent {

        /** Event pid */
        pid?: (number|null);

        /** Event type */
        type?: (string|null);

        /** Event comm */
        comm?: (string|null);

        /** Event path */
        path?: (string|null);
    }

    /** Represents an Event. */
    class Event implements IEvent {

        /**
         * Constructs a new Event.
         * @param [properties] Properties to set
         */
        constructor(properties?: pb.IEvent);

        /** Event pid. */
        public pid: number;

        /** Event type. */
        public type: string;

        /** Event comm. */
        public comm: string;

        /** Event path. */
        public path: string;

        /**
         * Creates a new Event instance using the specified properties.
         * @param [properties] Properties to set
         * @returns Event instance
         */
        public static create(properties?: pb.IEvent): pb.Event;

        /**
         * Encodes the specified Event message. Does not implicitly {@link pb.Event.verify|verify} messages.
         * @param message Event message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: pb.IEvent, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified Event message, length delimited. Does not implicitly {@link pb.Event.verify|verify} messages.
         * @param message Event message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: pb.IEvent, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an Event message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns Event
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): pb.Event;

        /**
         * Decodes an Event message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns Event
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): pb.Event;

        /**
         * Verifies an Event message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an Event message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns Event
         */
        public static fromObject(object: { [k: string]: any }): pb.Event;

        /**
         * Creates a plain object from an Event message. Also converts values to other types if specified.
         * @param message Event
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: pb.Event, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this Event to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for Event
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }
}
